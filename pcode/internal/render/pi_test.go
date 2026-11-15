// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const piHeader = `{"type":"session","version":3,"id":"0199aaaa-bbbb-7ccc-8ddd-eeeeffff0000","timestamp":"2026-11-15T15:00:00.000Z","cwd":"/repo"}`

func TestPiCompletedRun(t *testing.T) {
	input := piHeader + `
{"type":"agent_start"}
{"type":"turn_start"}
{"type":"message_end","message":{"role":"user","content":"list files"}}
{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Look"}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Looking."},{"type":"toolCall","id":"c1","name":"bash","arguments":{"command":"ls"}}],"stopReason":"toolUse"}}
{"type":"tool_execution_start","toolCallId":"c1","toolName":"bash","args":{"command":"ls -la"}}
{"type":"tool_execution_end","toolCallId":"c1","toolName":"bash","result":{"content":[{"type":"text","text":"a.go"}]},"isError":false}
{"type":"turn_end"}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Found a.go"}],"stopReason":"stop"}}
{"type":"agent_end","messages":[],"willRetry":false}
{"type":"agent_settled","aborted":false}
`
	var out bytes.Buffer
	completed := 0
	d := &PiDecoder{OnComplete: func() { completed++ }}
	res := d.Render(strings.NewReader(input), &out)
	if res.Err != nil || !res.Completed {
		t.Fatalf("res=%+v out=%s", res, out.String())
	}
	if res.SessionID != "0199aaaa-bbbb-7ccc-8ddd-eeeeffff0000" {
		t.Fatalf("session id = %q", res.SessionID)
	}
	if res.Text != "Found a.go" || completed != 1 {
		t.Fatalf("text=%q completed=%d", res.Text, completed)
	}
	got := out.String()
	for _, want := range []string{"pi agent started", "tool: shell: `ls -la`", "Looking.", "Found a.go", "pi agent completed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hmm") {
		t.Fatalf("thinking leaked into output:\n%s", got)
	}
	if d.Busy() {
		t.Fatal("expected no in-flight tools")
	}
}

func TestPiBusyDuringTool(t *testing.T) {
	d := &PiDecoder{}
	input := piHeader + `
{"type":"tool_execution_start","toolCallId":"c1","toolName":"bash","args":{"command":"sleep 999"}}
`
	var out bytes.Buffer
	res := d.Render(strings.NewReader(input), &out)
	if !d.Busy() {
		t.Fatal("expected busy while tool is running")
	}
	if !errors.Is(res.Err, ErrIncomplete) {
		t.Fatalf("expected incomplete, got %v", res.Err)
	}
}

func TestPiProviderErrorFailsRun(t *testing.T) {
	input := piHeader + `
{"type":"agent_start"}
{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"401 invalid api key"}}
{"type":"agent_end","messages":[],"willRetry":false}
{"type":"agent_settled","aborted":false}
`
	var out bytes.Buffer
	res := Pi(strings.NewReader(input), &out)
	if !errors.Is(res.Err, ErrAgent) || !strings.Contains(res.Err.Error(), "401 invalid api key") {
		t.Fatalf("err=%v out=%s", res.Err, out.String())
	}
	if !res.Completed || !strings.Contains(out.String(), "pi agent failed") {
		t.Fatalf("res=%+v out=%s", res, out.String())
	}
}

func TestPiRetryThenSuccess(t *testing.T) {
	input := piHeader + `
{"type":"agent_start"}
{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"529 overloaded"}}
{"type":"agent_end","messages":[],"willRetry":true}
{"type":"auto_retry_start","attempt":1,"maxAttempts":3,"delayMs":10,"errorMessage":"529 overloaded"}
{"type":"agent_start"}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"stopReason":"stop"}}
{"type":"auto_retry_end","success":true,"attempt":2}
{"type":"agent_end","messages":[],"willRetry":false}
{"type":"agent_settled","aborted":false}
`
	var out bytes.Buffer
	res := Pi(strings.NewReader(input), &out)
	if res.Err != nil || !res.Completed || res.Text != "done" {
		t.Fatalf("res=%+v out=%s", res, out.String())
	}
	if !strings.Contains(out.String(), "retrying (attempt 1/3)") {
		t.Fatalf("missing retry line: %s", out.String())
	}
}

func TestPiAborted(t *testing.T) {
	input := piHeader + `
{"type":"agent_start"}
{"type":"agent_settled","aborted":true}
`
	res := Pi(strings.NewReader(input), &bytes.Buffer{})
	if !errors.Is(res.Err, ErrAgent) {
		t.Fatalf("expected agent error, got %v", res.Err)
	}
}

func TestPiToolFailurePrinted(t *testing.T) {
	input := piHeader + `
{"type":"tool_execution_start","toolCallId":"c1","toolName":"edit","args":{"path":"a.go"}}
{"type":"tool_execution_end","toolCallId":"c1","toolName":"edit","result":{"content":[{"type":"text","text":"oldText not found"}]},"isError":true}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"fixed"}],"stopReason":"stop"}}
{"type":"agent_settled","aborted":false}
`
	var out bytes.Buffer
	res := Pi(strings.NewReader(input), &out)
	if res.Err != nil {
		t.Fatalf("tool failure must not fail the run: %v", res.Err)
	}
	for _, want := range []string{"tool: edit a.go", "tool failed: edit: oldText not found"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, out.String())
		}
	}
}

func TestPiNoEvents(t *testing.T) {
	res := Pi(strings.NewReader("No API key found for fireworks.\n"), &bytes.Buffer{})
	if !errors.Is(res.Err, ErrIncomplete) || res.Started {
		t.Fatalf("res=%+v", res)
	}
}

func TestPiAgentEndWithoutSettled(t *testing.T) {
	input := piHeader + `
{"type":"agent_start"}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"stopReason":"stop"}}
{"type":"agent_end","messages":[],"willRetry":false}
`
	res := Pi(strings.NewReader(input), &bytes.Buffer{})
	if res.Err != nil || !res.Completed {
		t.Fatalf("res=%+v", res)
	}
}

func TestPiOversizedLineSkipped(t *testing.T) {
	huge := `{"type":"agent_end","messages":[{"role":"toolResult","content":"` + strings.Repeat("x", piMaxLine+10) + `"}],"willRetry":false}`
	input := piHeader + `
{"type":"message_start","message":{"role":"assistant","provider":"fireworks","model":"kimi","content":[]}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"big"}],"stopReason":"stop"}}
` + huge + `
{"type":"agent_settled","aborted":false}`
	var out bytes.Buffer
	res := Pi(strings.NewReader(input), &out)
	if res.Err != nil || !res.Completed || res.Text != "big" {
		t.Fatalf("res=%+v", res)
	}
	if !strings.Contains(out.String(), "model: fireworks/kimi") {
		t.Fatalf("missing model line: %s", out.String())
	}
}

func TestPiCRLF(t *testing.T) {
	input := strings.ReplaceAll(piHeader+"\n"+`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"stopReason":"stop"}}`+"\n"+`{"type":"agent_settled","aborted":false}`+"\n", "\n", "\r\n")
	res := Pi(strings.NewReader(input), &bytes.Buffer{})
	if res.Err != nil || res.Text != "ok" {
		t.Fatalf("res=%+v", res)
	}
}
