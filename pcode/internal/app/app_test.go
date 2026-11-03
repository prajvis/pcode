// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prajvis/pcode/pcode/internal/config"
	"github.com/prajvis/pcode/pcode/internal/execx"
	"github.com/prajvis/pcode/pcode/internal/registry"
	"github.com/prajvis/pcode/pcode/internal/remote"
	"github.com/prajvis/pcode/pcode/internal/threads"
)

type fakeSession struct {
	mu        sync.Mutex
	Commands  []string
	handler   func(command string, stdout, stderr io.Writer) error
	cancelled atomic.Bool
}

func defaultFakeHandler(command string, stdout, stderr io.Writer) error {
	if isRunningScriptCommand(command) {
		fmt.Fprintln(stdout, execx.RunStatusIdle)
	}
	return nil
}

func (s *fakeSession) Run(ctx context.Context, command string, stdout, stderr io.Writer) error {
	s.mu.Lock()
	s.Commands = append(s.Commands, command)
	s.mu.Unlock()
	handler := s.handler
	if handler == nil {
		handler = defaultFakeHandler
	} else {
		inner := handler
		handler = func(command string, stdout, stderr io.Writer) error {
			var captured bytes.Buffer
			err := inner(command, io.MultiWriter(stdout, &captured), stderr)
			if err != nil {
				return err
			}
			if isRunningScriptCommand(command) && len(bytes.TrimSpace(captured.Bytes())) == 0 {
				fmt.Fprintln(stdout, execx.RunStatusIdle)
			}
			return nil
		}
	}
	errCh := make(chan error, 1)
	go func() { errCh <- handler(command, stdout, stderr) }()
	select {
	case <-ctx.Done():
		s.cancelled.Store(true)
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *fakeSession) RunWithStdin(ctx context.Context, command string, stdin io.Reader, stdout, stderr io.Writer) error {
	if stdin != nil {
		go func() { _, _ = io.Copy(io.Discard, stdin) }()
	}
	return s.Run(ctx, command, stdout, stderr)
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
			if strings.Contains(command, "agent") {
				fmt.Fprintln(stdout, `{"type":"system","model":"test"}`)
				fmt.Fprintln(stdout, `{"type":"result","result":"ping-ok"}`)
				return nil
			}
			return nil
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
	if !strings.Contains(joined, "--resume") || !strings.Contains(joined, chatID) {
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

func TestRunCursorIdleTimeoutCancels(t *testing.T) {
	const chatID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if strings.Contains(command, "create-chat") {
				fmt.Fprintln(stdout, chatID)
				return nil
			}
			if !strings.Contains(command, "agent") {
				return nil
			}
			fmt.Fprintln(stdout, `{"type":"system","model":"test"}`)
			fmt.Fprintln(stdout, `{"type":"tool_call","subtype":"started","tool_call":{"shellToolCall":{"args":{"command":"echo hi"}}}}`)
			fmt.Fprintln(stdout, `{"type":"tool_call","subtype":"completed","tool_call":{"shellToolCall":{"result":{"success":{"stdout":"hi"}}}}}`)
			time.Sleep(200 * time.Millisecond)
			for {
				if _, err := fmt.Fprintln(stdout, `{"type":"assistant","message":{"content":"still going"}}`); err != nil {
					return err
				}
				time.Sleep(20 * time.Millisecond)
			}
		},
	}
	app, _, _ := testApp(t, sess)
	app.Cfg.IdleTimeout = 50 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		done <- app.Run(context.Background(), []string{"website", "ai", "idle", "hello"})
	}()
	select {
	case err := <-done:
		if err == nil || !errors.Is(err, execx.ErrIdleTimeout) {
			t.Fatalf("err=%v", err)
		}
		if !sess.cancelled.Load() {
			t.Fatal("expected fake session to observe cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runCursor did not return within 2s after idle timeout")
	}
}

func TestRunCursorBusySkipsIdle(t *testing.T) {
	const chatID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if strings.Contains(command, "create-chat") {
				fmt.Fprintln(stdout, chatID)
				return nil
			}
			if !strings.Contains(command, "agent") {
				return nil
			}
			fmt.Fprintln(stdout, `{"type":"system","model":"test"}`)
			fmt.Fprintln(stdout, `{"type":"tool_call","subtype":"started","tool_call":{"shellToolCall":{"args":{"command":"sleep 1"}}}}`)
			time.Sleep(200 * time.Millisecond)
			fmt.Fprintln(stdout, `{"type":"tool_call","subtype":"completed","tool_call":{"shellToolCall":{"result":{"success":{}}}}}`)
			fmt.Fprintln(stdout, `{"type":"result","result":"ok"}`)
			return nil
		},
	}
	app, _, _ := testApp(t, sess)
	app.Cfg.IdleTimeout = 50 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		done <- app.Run(context.Background(), []string{"website", "ai", "busy", "hello"})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runCursor did not return within 2s while a tool call was in flight")
	}
}

func TestAIIdleIsRunningAllowsRun(t *testing.T) {
	const chatID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if isRunningScriptCommand(command) {
				fmt.Fprintln(stdout, "NOT_RUNNING")
				return nil
			}
			if strings.Contains(command, "create-chat") {
				fmt.Fprintln(stdout, chatID)
				return nil
			}
			if strings.Contains(command, "agent") {
				fmt.Fprintln(stdout, `{"type":"system","model":"test"}`)
				fmt.Fprintln(stdout, `{"type":"result","result":"ok"}`)
				return nil
			}
			return nil
		},
	}
	app, _, _ := testApp(t, sess)
	if err := app.Run(context.Background(), []string{"website", "ai", "fresh", "hello"}); err != nil {
		t.Fatalf("ai should proceed when idle: %v", err)
	}
}

func TestAIActiveIsRunningRefuses(t *testing.T) {
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if isRunningScriptCommand(command) {
				fmt.Fprintln(stdout, "PCODE_RUN_ACTIVE")
				return nil
			}
			return fmt.Errorf("ai should not start remote work")
		},
	}
	app, _, _ := testApp(t, sess)
	err := app.Run(context.Background(), []string{"website", "ai", "fresh", "hello"})
	if err == nil || !strings.Contains(err.Error(), "pcode website stop fresh") {
		t.Fatalf("err=%v", err)
	}
}

func isRunningScriptCommand(command string) bool {
	if !strings.Contains(command, "$HOME/.pcode/runs/") {
		return false
	}
	return strings.Contains(command, "NOT_RUNNING") ||
		strings.Contains(command, "PCODE_RUN_IDLE") ||
		strings.Contains(command, "PCODE_RUN_ACTIVE") ||
		strings.Contains(command, "echo RUNNING")
}

func TestRunCursorReturnsAfterResultWithoutEOF(t *testing.T) {
	const chatID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if isRunningScriptCommand(command) {
				fmt.Fprintln(stdout, execx.RunStatusIdle)
				return nil
			}
			if strings.Contains(command, "create-chat") {
				fmt.Fprintln(stdout, chatID)
				return nil
			}
			if !strings.Contains(command, "agent") {
				return nil
			}
			fmt.Fprintln(stdout, `{"type":"system","model":"test"}`)
			fmt.Fprintln(stdout, `{"type":"result","result":"ok"}`)
			select {
			case <-time.After(30 * time.Second):
				return nil
			}
		},
	}
	app, _, _ := testApp(t, sess)
	app.Cfg.StreamGrace = 80 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		done <- app.Run(context.Background(), []string{"website", "ai", "hang", "hello"})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(80*time.Millisecond + time.Second):
		t.Fatal("runCursor did not return within grace+1s after result without EOF")
	}
}
