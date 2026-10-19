// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type CursorResult struct {
	Started   bool
	Completed bool
	Text      string
	Err       error
}

func Cursor(r io.Reader, w io.Writer) CursorResult {
	res := CursorResult{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	printedTools := map[string]bool{}
	var lastAssistant string
	lastEvent := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		etype := str(pick(event, "type", "event", "kind"))
		subtype := str(event["subtype"])
		lastEvent = etype
		if subtype != "" {
			lastEvent += "/" + subtype
		}

		switch {
		case in(etype, "system", "session_start", "started", "start"):
			if !res.Started {
				model := str(pick(event, "model", "modelName"))
				msg := "cursor agent started"
				if model != "" {
					msg += " (" + model + ")"
				}
				fmt.Fprintln(w, msg)
				res.Started = true
			}
		case in(etype, "tool_call", "tool_use", "tool", "function_call"):
			if in(subtype, "completed", "done") {
				if failure := toolFailure(event); failure != "" {
					fmt.Fprintf(w, "error: %s\n", failure)
					res.Err = fmt.Errorf("%s", failure)
				}
				continue
			}
			if subtype != "" && !in(subtype, "started", "start") {
				continue
			}
			label := toolLabel(event)
			if !printedTools[label] {
				fmt.Fprintf(w, "tool: %s\n", label)
				printedTools[label] = true
			}
		case in(etype, "error", "failed", "failure"):
			msg := extractText(pick(event, "message", "error"))
			if msg == "" {
				msg = line
			}
			fmt.Fprintf(w, "error: %s\n", msg)
			res.Err = fmt.Errorf("%s", msg)
		case in(etype, "result", "final", "done", "completed", "completion"):
			if boolish(event["is_error"]) || in(subtype, "error", "failed", "failure", "cancelled", "canceled", "aborted") {
				res.Err = fmt.Errorf("cursor agent error")
			}
			text := extractText(pick(event, "result", "text", "content", "message"))
			printFinal(w, &res, text)
		case in(etype, "cancelled", "canceled", "aborted") || in(subtype, "cancelled", "canceled", "aborted"):
			msg := extractText(pick(event, "message", "error", "result"))
			if msg == "" {
				msg = lastEvent
			}
			printIncomplete(w, &res, lastAssistant, msg)
		case etype == "assistant":
			if text := extractText(event["message"]); text != "" {
				lastAssistant = text
			}
		}
	}
	if err := scanner.Err(); err != nil && res.Err == nil {
		res.Err = err
	}
	if res.Started && !res.Completed {
		printIncomplete(w, &res, lastAssistant, "stream ended after "+or(lastEvent, "unknown event"))
	}
	if !res.Started && !res.Completed {
		printIncomplete(w, &res, "", "no stream events received from cursor agent")
	}
	return res
}

func printFinal(w io.Writer, res *CursorResult, text string) {
	text = strings.TrimSpace(text)
	if text != "" {
		fmt.Fprintf(w, "\n%s\n", text)
		res.Text = text
	}
	if !res.Completed {
		fmt.Fprintln(w, "cursor agent completed")
		res.Completed = true
	}
}

func printIncomplete(w io.Writer, res *CursorResult, last, reason string) {
	last = strings.TrimSpace(last)
	if last != "" {
		fmt.Fprintf(w, "\n%s\n", last)
		res.Text = last
	}
	if !res.Completed {
		fmt.Fprintf(w, "cursor agent incomplete: %s\n", reason)
		res.Completed = true
	}
	if res.Err == nil {
		res.Err = fmt.Errorf("cursor agent incomplete: %s", reason)
	}
}

func pick(obj map[string]any, names ...string) any {
	for _, name := range names {
		if v, ok := obj[name]; ok && v != nil && v != "" {
			return v
		}
	}
	return nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func boolish(v any) bool {
	b, _ := v.(bool)
	return b
}

func in(v string, names ...string) bool {
	for _, name := range names {
		if v == name {
			return true
		}
	}
	return false
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func extractText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		if str(v["role"]) == "assistant" {
			if text := extractText(v["content"]); text != "" {
				return text
			}
		}
		for _, key := range []string{"text", "result", "summary", "content", "message"} {
			if text := extractText(v[key]); text != "" {
				return text
			}
		}
	case []any:
		var parts []string
		for _, item := range v {
			if text := extractText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

func toolLabel(event map[string]any) string {
	call, _ := event["tool_call"].(map[string]any)
	if call == nil {
		call, _ = event["toolCall"].(map[string]any)
	}
	if call != nil {
		if shell, ok := call["shellToolCall"].(map[string]any); ok {
			if args, ok := shell["args"].(map[string]any); ok {
				cmd := str(args["command"])
				if cmd != "" {
					return "shell: `" + cmd + "`"
				}
			}
		}
		for _, key := range []string{"name", "tool", "tool_name", "functionName"} {
			if s := str(call[key]); s != "" {
				return s
			}
		}
	}
	for _, key := range []string{"name", "tool", "tool_name"} {
		if s := str(event[key]); s != "" {
			return s
		}
	}
	return "tool"
}

func toolFailure(event map[string]any) string {
	call, _ := event["tool_call"].(map[string]any)
	if call == nil {
		return ""
	}
	for key, value := range call {
		inner, ok := value.(map[string]any)
		if !ok || !strings.HasSuffix(key, "ToolCall") {
			continue
		}
		result, _ := inner["result"].(map[string]any)
		if result == nil {
			continue
		}
		if failure, ok := result["failure"].(map[string]any); ok {
			msg := str(failure["stderr"])
			if msg == "" {
				msg = str(failure["interleavedOutput"])
			}
			return strings.TrimSuffix(key, "ToolCall") + " failed: " + strings.TrimSpace(msg)
		}
	}
	return ""
}
