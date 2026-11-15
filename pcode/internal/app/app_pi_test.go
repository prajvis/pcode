// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/prajvis/pcode/pcode/internal/config"
	"github.com/prajvis/pcode/pcode/internal/registry"
	"github.com/prajvis/pcode/pcode/internal/render"
)

const piOKStream = `{"type":"session","version":3,"id":"%s","cwd":"/home/coder/website"}
{"type":"agent_start"}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"pong"}],"stopReason":"stop"}}
{"type":"agent_end","messages":[],"willRetry":false}
{"type":"agent_settled","aborted":false}
`

func piApp(t *testing.T, sess *fakeSession) *App {
	t.Helper()
	app, _, _ := testApp(t, sess)
	app.Cfg.PiCommand = "pi"
	if err := app.Reg.Upsert(registry.Record{
		Alias:     "website",
		Workspace: "my-website",
		Repo:      "/home/coder/website",
		AICLI:     "pi",
	}); err != nil {
		t.Fatal(err)
	}
	return app
}

func piSessionID(t *testing.T, command string) string {
	t.Helper()
	id, ok := ExtractUUID(command[strings.Index(command, "--session-id"):])
	if !ok {
		t.Fatalf("no session id in %s", command)
	}
	return id
}

func TestAIPiCreatesThenResumesSameSession(t *testing.T) {
	var runs []string
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if strings.Contains(command, "pi --mode json") {
				runs = append(runs, command)
				fmt.Fprintf(stdout, piOKStream, piSessionID(t, command))
			}
			return nil
		},
	}
	app := piApp(t, sess)
	ctx := context.Background()
	if err := app.Run(ctx, []string{"website", "ai", "ping", "-- reply pong"}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := app.Run(ctx, []string{"website", "ai", "ping", "again"}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 pi runs, got %d", len(runs))
	}
	first, second := piSessionID(t, runs[0]), piSessionID(t, runs[1])
	if first != second {
		t.Fatalf("expected same session id, got %s then %s", first, second)
	}
	if strings.Contains(runs[0], "create-chat") || strings.Contains(runs[0], "--resume") {
		t.Fatalf("unexpected cursor flags: %s", runs[0])
	}
	meta, err := app.Threads.Read("website", "ping")
	if err != nil || meta.ThreadID != first || meta.AICLI != "pi" {
		t.Fatalf("meta=%+v err=%v", meta, err)
	}
}

func TestAIPiAgentErrorExitCode(t *testing.T) {
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if strings.Contains(command, "pi --mode json") {
				fmt.Fprintln(stdout, `{"type":"session","version":3,"id":"x"}`)
				fmt.Fprintln(stdout, `{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"401 bad key"}}`)
				fmt.Fprintln(stdout, `{"type":"agent_settled","aborted":false}`)
			}
			return nil
		},
	}
	app := piApp(t, sess)
	err := app.Run(context.Background(), []string{"website", "ai", "bad", "hi"})
	if !errors.Is(err, render.ErrAgent) || ExitCode(err) != 2 {
		t.Fatalf("err=%v code=%d", err, ExitCode(err))
	}
	// The session id is kept so a retry continues the same Pi session.
	if meta, err := app.Threads.Read("website", "bad"); err != nil || meta.ThreadID == "" {
		t.Fatalf("expected stored thread after failure: %+v %v", meta, err)
	}
}

func TestAIPiNoEventsIncomplete(t *testing.T) {
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if strings.Contains(command, "pi --mode json") {
				fmt.Fprintln(stderr, "No API key found for fireworks.")
				return fmt.Errorf("exit status 1")
			}
			return nil
		},
	}
	app := piApp(t, sess)
	err := app.Run(context.Background(), []string{"website", "ai", "nokey", "hi"})
	if err == nil || ExitCode(err) == 0 {
		t.Fatalf("expected failure, got %v", err)
	}
}

func TestPiCommandLine(t *testing.T) {
	cfg := config.Config{PiCommand: "pi", PiArgs: "--model fireworks/kimi --approve"}
	got := PiCommandLine(cfg, "/home/coder/my repo", "it's -x", "abc")
	want := `cd '/home/coder/my repo' && pi --mode json --session-id 'abc' --model fireworks/kimi --approve -- 'it'"'"'s -x'`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestNewUUID(t *testing.T) {
	a, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewUUID()
	if a == b {
		t.Fatal("expected distinct ids")
	}
	if id, ok := ExtractUUID(a); !ok || id != a || a[14] != '4' {
		t.Fatalf("bad uuid %q", a)
	}
}

func TestNormalizePi(t *testing.T) {
	for _, in := range []string{"pi", "pi-agent", "pi-coding-agent"} {
		if got := config.NormalizeAICLI(in); got != "pi" {
			t.Fatalf("%s -> %s", in, got)
		}
	}
	if !config.ThreadBacked("pi") {
		t.Fatal("pi should be thread backed")
	}
}

func TestAIIgnoresThreadFromOtherBackend(t *testing.T) {
	const cursorChat = "11111111-2222-3333-4444-555555555555"
	var runs []string
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			if strings.Contains(command, "pi --mode json") {
				runs = append(runs, command)
				fmt.Fprintf(stdout, piOKStream, piSessionID(t, command))
			}
			return nil
		},
	}
	app := piApp(t, sess)
	if err := app.Threads.Write("website", "shared", "cursor", "my-website", "/home/coder/website", cursorChat); err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background(), []string{"website", "ai", "shared", "hi"}); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || strings.Contains(runs[0], cursorChat) {
		t.Fatalf("cursor chat id reused for pi: %v", runs)
	}
	meta, err := app.Threads.Read("website", "shared")
	if err != nil || meta.AICLI != "pi" || meta.ThreadID == cursorChat {
		t.Fatalf("meta=%+v err=%v", meta, err)
	}
}

// The alias stays cursor; --ai-cli pi runs pi once, and each backend keeps
// its own conversation under the same chat title.
func TestAICLIOverridePerCall(t *testing.T) {
	const cursorChat = "11111111-2222-3333-4444-555555555555"
	var cursorRuns, piRuns []string
	sess := &fakeSession{
		handler: func(command string, stdout, stderr io.Writer) error {
			switch {
			case strings.Contains(command, "create-chat"):
				fmt.Fprintln(stdout, "Created chat "+cursorChat)
			case strings.Contains(command, "pi --mode json"):
				piRuns = append(piRuns, command)
				fmt.Fprintf(stdout, piOKStream, piSessionID(t, command))
			case strings.Contains(command, "agent -p"):
				cursorRuns = append(cursorRuns, command)
				fmt.Fprintln(stdout, `{"type":"system","model":"test"}`)
				fmt.Fprintln(stdout, `{"type":"result","result":"ok"}`)
			}
			return nil
		},
	}
	app, _, stderr := testApp(t, sess) // alias "website" registered as cursor
	app.Cfg.PiCommand = "pi"
	ctx := context.Background()
	if err := app.Run(ctx, []string{"website", "ai", "fix", "hi"}); err != nil {
		t.Fatalf("cursor run: %v", err)
	}
	if err := app.Run(ctx, []string{"website", "ai", "--ai-cli", "pi", "fix", "hi"}); err != nil {
		t.Fatalf("pi run: %v", err)
	}
	if err := app.Run(ctx, []string{"website", "ai", "--ai-cli=pi", "fix", "again"}); err != nil {
		t.Fatalf("pi follow-up: %v", err)
	}
	if err := app.Run(ctx, []string{"website", "ai", "fix", "back to cursor"}); err != nil {
		t.Fatalf("cursor follow-up: %v", err)
	}
	if len(cursorRuns) != 2 || len(piRuns) != 2 {
		t.Fatalf("cursor=%d pi=%d", len(cursorRuns), len(piRuns))
	}
	if piSessionID(t, piRuns[0]) != piSessionID(t, piRuns[1]) || strings.Contains(piRuns[0], cursorChat) {
		t.Fatalf("pi session not reused or cursor id leaked: %v", piRuns)
	}
	if !strings.Contains(cursorRuns[1], cursorChat) {
		t.Fatalf("cursor chat not resumed after pi runs: %s", cursorRuns[1])
	}
	rec, err := app.Reg.Get("website")
	if err != nil || rec.AICLI != "cursor" {
		t.Fatalf("override must not change the alias: %+v %v", rec, err)
	}
	if !strings.Contains(stderr.String(), "resuming chat fix (") || !strings.Contains(stderr.String(), ", pi)") {
		t.Fatalf("stderr: %s", stderr.String())
	}
}
