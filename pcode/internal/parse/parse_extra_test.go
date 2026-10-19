// SPDX-License-Identifier: Apache-2.0

package parse

import "testing"

func TestParseUnknownCommand(t *testing.T) {
	if _, err := Parse([]string{"website", "follow", "x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseShellAndThreads(t *testing.T) {
	req, err := Parse([]string{"website", "shell"})
	if err != nil || req.Kind != KindShell {
		t.Fatalf("%+v %v", req, err)
	}
	req, err = Parse([]string{"website", "threads"})
	if err != nil || req.Kind != KindThreads {
		t.Fatalf("%+v %v", req, err)
	}
}
