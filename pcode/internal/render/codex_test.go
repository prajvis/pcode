// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestCodexErrorThenCompleted(t *testing.T) {
	var out bytes.Buffer
	input := `{"type":"thread.started","thread_id":"t1"}
{"type":"error","message":"transient"}
{"type":"turn.completed"}
`
	res := Codex(strings.NewReader(input), &out)
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
