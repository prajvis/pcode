// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type CodexResult struct {
	Started   bool
	Completed bool
	ThreadID  string
	Err       error
}

func Codex(r io.Reader, w io.Writer) CodexResult {
	res := CodexResult{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		switch str(event["type"]) {
		case "thread.started":
			res.ThreadID = str(event["thread_id"])
			if !res.Started {
				fmt.Fprintln(w, "codex agent started")
				res.Started = true
			}
		case "item.completed":
			item, _ := event["item"].(map[string]any)
			if str(item["type"]) == "agent_message" {
				text := strings.TrimSpace(str(item["text"]))
				if text != "" {
					fmt.Fprintf(w, "\n%s\n", text)
				}
			}
		case "turn.completed":
			if !res.Completed {
				fmt.Fprintln(w, "codex agent completed")
				res.Completed = true
			}
		case "error":
			msg := str(event["message"])
			if msg == "" {
				msg = line
			}
			fmt.Fprintf(w, "error: %s\n", msg)
			res.Err = fmt.Errorf("%s", msg)
		}
	}
	if err := scanner.Err(); err != nil && res.Err == nil {
		res.Err = err
	}
	if res.Started && !res.Completed {
		fmt.Fprintln(w, "codex agent completed")
		res.Completed = true
	}
	if !res.Started && !res.Completed {
		res.Err = fmt.Errorf("no stream events received from codex agent")
	}
	return res
}
