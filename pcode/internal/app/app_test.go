// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/prajvis/pcode/pcode/internal/config"
	"github.com/prajvis/pcode/pcode/internal/registry"
	"github.com/prajvis/pcode/pcode/internal/remote"
	"github.com/prajvis/pcode/pcode/internal/threads"
)

type fakeSession struct {
	mu       sync.Mutex
	Commands []string
	handler  func(command string, stdout, stderr io.Writer) error
}

func (s *fakeSession) Run(_ context.Context, command string, stdout, stderr io.Writer) error {
	s.mu.Lock()
	s.Commands = append(s.Commands, command)
	s.mu.Unlock()
	if s.handler != nil {
		return s.handler(command, stdout, stderr)
	}
	return nil
}

func (s *fakeSession) Output(ctx context.Context, command string) (string, error) {
	var buf bytes.Buffer
	err := s.Run(ctx, command, &buf, io.Discard)
	return buf.String(), err
}

func (s *fakeSession) Shell(context.Context) error { return nil }
func (s *fakeSession) Close() error                { return nil }

func testApp(t *testing.T, sess *fakeSession) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		RegistryFile:  filepath.Join(dir, "workspaces.tsv"),
		AIStateDir:    filepath.Join(dir, "threads"),
		CursorCommand: "agent",
		CodexCommand:  "codex",
		ClaudeCommand: "claude",
		CursorStream:  true,
		CursorForce:   true,
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := &App{
		Cfg:     cfg,
		Reg:     registry.Open(cfg.RegistryFile),
		Threads: &threads.Store{Root: cfg.AIStateDir},
		Stdout:  stdout,
		Stderr:  stderr,
		Dial: func(context.Context, string) (remote.Session, error) {
			return sess, nil
		},
	}
	if err := app.Reg.Upsert(registry.Record{
		Alias:     "website",
		Workspace: "my-website",
		Repo:      "/home/coder/website",
		AICLI:     "cursor",
	}); err != nil {
		t.Fatal(err)
	}
	return app, stdout, stderr
}

func TestAICreatesThenAutoFollows(t *testing.T) {
	const chatID = "11111111-2222-3333-4444-555555555555"
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if strings.Contains(command, "create-chat") {
				fmt.Fprintln(stdout, "Created chat "+chatID)
				return nil
			}
			if strings.Contains(command, "agent -p") {
				fmt.Fprintln(stdout, `{"type":"system","model":"test"}`)
				fmt.Fprintln(stdout, `{"type":"result","result":"ping-ok"}`)
				return nil
			}
			return fmt.Errorf("unexpected command: %s", command)
		},
	}
	app, _, stderr := testApp(t, sess)
	ctx := context.Background()
	if err := app.Run(ctx, []string{"website", "ai", "ping", "Reply: ping-ok"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "creating cursor chat") {
		t.Fatalf("stderr=%s", stderr.String())
	}
	meta, err := app.Threads.Read("website", "ping")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ThreadID != chatID {
		t.Fatalf("thread=%q", meta.ThreadID)
	}

	stderr.Reset()
	sess.Commands = nil
	if err := app.Run(ctx, []string{"website", "ai", "ping", "again"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "resuming chat ping") {
		t.Fatalf("stderr=%s", stderr.String())
	}
	joined := strings.Join(sess.Commands, "\n")
	if strings.Contains(joined, "create-chat") {
		t.Fatalf("follow-up should not create a chat: %s", joined)
	}
	if !strings.Contains(joined, "--resume '"+chatID+"'") {
		t.Fatalf("missing resume: %s", joined)
	}
}

func TestAIEmptyStreamFails(t *testing.T) {
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if strings.Contains(command, "create-chat") {
				fmt.Fprintln(stdout, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
				return nil
			}
			return nil
		},
	}
	app, _, _ := testApp(t, sess)
	err := app.Run(context.Background(), []string{"website", "ai", "blank", "hello"})
	if err == nil || !strings.Contains(err.Error(), "no stream events") {
		t.Fatalf("err=%v", err)
	}
}

func TestExecRunsInRepo(t *testing.T) {
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			fmt.Fprintln(stdout, "/home/coder/website")
			return nil
		},
	}
	app, stdout, _ := testApp(t, sess)
	if err := app.Run(context.Background(), []string{"website", "exec", "pwd"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sess.Commands[0], "cd '/home/coder/website' && pwd") {
		t.Fatalf("cmd=%s", sess.Commands[0])
	}
	if !strings.Contains(stdout.String(), "/home/coder/website") {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestRemoveDropsAlias(t *testing.T) {
	app, stdout, _ := testApp(t, &fakeSession{})
	if err := app.Run(context.Background(), []string{"rm", "website"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "removed alias website") {
		t.Fatalf("stdout=%s", stdout.String())
	}
	if _, err := app.Reg.Get("website"); err == nil {
		t.Fatal("expected unknown alias")
	}
	if err := app.Run(context.Background(), []string{"website", "exec", "pwd"}); err == nil {
		t.Fatal("expected exec to fail after remove")
	}
}

func TestExtractUUID(t *testing.T) {
	id, ok := ExtractUUID("Created chat 11111111-2222-3333-4444-555555555555 ok")
	if !ok || id != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("%q %v", id, ok)
	}
	if _, ok := ExtractUUID("nope"); ok {
		t.Fatal("expected miss")
	}
}
