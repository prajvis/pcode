// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestCursorEmptyStreamFails(t *testing.T) {
	var out bytes.Buffer
	res := Cursor(strings.NewReader(""), &out)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "no stream events") {
		t.Fatalf("err=%v out=%s", res.Err, out.String())
	}
}

func TestCursorCompletes(t *testing.T) {
	var out bytes.Buffer
	input := `{"type":"system","model":"x"}
{"type":"result","result":"done"}
`
	res := Cursor(strings.NewReader(input), &out)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if !res.Started || !res.Completed {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(out.String(), "done") {
		t.Fatalf("out=%s", out.String())
	}
}

func TestCursorFailedToolThenSuccess(t *testing.T) {
	var out bytes.Buffer
	input := `{"type":"system","model":"x"}
{"type":"tool_call","subtype":"completed","tool_call":{"shellToolCall":{"result":{"failure":{"stderr":"no matches"}}}}}
{"type":"result","result":"OK"}
`
	res := Cursor(strings.NewReader(input), &out)
	if res.Err != nil {
		t.Fatalf("Err=%v out=%s", res.Err, out.String())
	}
	if !res.Completed {
		t.Fatalf("expected completed: %+v", res)
	}
	if !strings.Contains(out.String(), "tool failed:") {
		t.Fatalf("expected tool failed warning, out=%s", out.String())
	}
}

func TestCursorResultIsError(t *testing.T) {
	var out bytes.Buffer
	input := `{"type":"system","model":"x"}
{"type":"result","is_error":true,"result":"boom"}
`
	res := Cursor(strings.NewReader(input), &out)
	if res.Err == nil {
		t.Fatalf("expected error, out=%s", out.String())
	}
}

func TestCursorNoResultIncomplete(t *testing.T) {
	var out bytes.Buffer
	input := `{"type":"system","model":"x"}
{"type":"tool_call","subtype":"started","tool_call":{"shellToolCall":{"args":{"command":"true"}}}}
`
	res := Cursor(strings.NewReader(input), &out)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "incomplete") {
		t.Fatalf("err=%v out=%s", res.Err, out.String())
	}
}

func TestCursorErrorThenSuccess(t *testing.T) {
	var out bytes.Buffer
	input := `{"type":"system","model":"x"}
{"type":"error","message":"transient"}
{"type":"result","result":"recovered"}
`
	res := Cursor(strings.NewReader(input), &out)
	if res.Err != nil {
		t.Fatalf("Err=%v out=%s", res.Err, out.String())
	}
	if !res.Completed {
		t.Fatalf("expected completed: %+v", res)
	}
	if !strings.Contains(out.String(), "transient") {
		t.Fatalf("expected printed error, out=%s", out.String())
	}
}
