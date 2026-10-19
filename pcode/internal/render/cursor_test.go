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
