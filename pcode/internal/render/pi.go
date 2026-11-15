// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// PiResult is the outcome of rendering a `pi --mode json` event stream.
type PiResult struct {
	Started   bool
	Completed bool
	SessionID string
	Text      string
	Err       error
}

// PiDecoder renders a Pi JSONL event stream (pi --mode json) and tracks
// in-flight tool executions so idle detection does not fire during long
// tool calls.
//
// Event reference: packages/coding-agent/docs/json.md in earendil-works/pi.
type PiDecoder struct {
	mu         sync.Mutex
	inflight   map[string]bool
	OnComplete func()
}

func (d *PiDecoder) Busy() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.inflight) > 0
}

func (d *PiDecoder) toolStart(id string) {
	d.mu.Lock()
	if d.inflight == nil {
		d.inflight = map[string]bool{}
	}
	d.inflight[id] = true
	d.mu.Unlock()
}

func (d *PiDecoder) toolEnd(id string) {
	d.mu.Lock()
	delete(d.inflight, id)
	d.mu.Unlock()
}

func (d *PiDecoder) clearTools() {
	d.mu.Lock()
	d.inflight = nil
	d.mu.Unlock()
}

func Pi(r io.Reader, w io.Writer) PiResult {
	return new(PiDecoder).Render(r, w)
}

func (d *PiDecoder) Render(r io.Reader, w io.Writer) PiResult {
	if d == nil {
		d = new(PiDecoder)
	}
	res := PiResult{}
	lines := &piLineReader{r: bufio.NewReaderSize(r, 64*1024), max: piMaxLine}
	var readErr error
	printedModel := false

	settled := false
	agentEnded := false
	lastEvent := ""
	// lastFailure is the error of the most recent failed assistant message or
	// retry. A later successful assistant message clears it.
	lastFailure := ""

	for {
		raw, skipped, err := lines.next()
		if err != nil {
			if err != io.EOF {
				readErr = err
			}
			break
		}
		if skipped {
			// Oversized record (typically agent_end, which repeats every
			// message of the run). Drop it and keep reading.
			continue
		}
		line := strings.TrimSpace(string(raw))
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		etype := str(event["type"])
		if etype != "message_update" && etype != "tool_execution_update" {
			lastEvent = etype
		}

		switch etype {
		case "session":
			res.SessionID = str(event["id"])
			if !res.Started {
				fmt.Fprintln(w, "pi agent started")
				res.Started = true
			}
		case "agent_start":
			agentEnded = false
			if !res.Started {
				fmt.Fprintln(w, "pi agent started")
				res.Started = true
			}
		case "message_start":
			msg, _ := event["message"].(map[string]any)
			if msg != nil && str(msg["role"]) == "assistant" && !printedModel {
				if model := str(msg["model"]); model != "" {
					if p := str(msg["provider"]); p != "" {
						model = p + "/" + model
					}
					fmt.Fprintf(w, "model: %s\n", model)
					printedModel = true
				}
			}
		case "tool_execution_start":
			d.toolStart(str(event["toolCallId"]))
			fmt.Fprintf(w, "tool: %s\n", piToolLabel(str(event["toolName"]), event["args"]))
		case "tool_execution_end":
			d.toolEnd(str(event["toolCallId"]))
			if boolish(event["isError"]) {
				msg := truncate(strings.TrimSpace(extractText(event["result"])), 500)
				fmt.Fprintf(w, "tool failed: %s: %s\n", or(str(event["toolName"]), "tool"), msg)
			}
		case "message_end":
			msg, _ := event["message"].(map[string]any)
			if msg == nil || str(msg["role"]) != "assistant" {
				continue
			}
			switch str(msg["stopReason"]) {
			case "error", "aborted":
				lastFailure = or(strings.TrimSpace(str(msg["errorMessage"])), "assistant message "+str(msg["stopReason"]))
				fmt.Fprintf(w, "error: %s\n", lastFailure)
			default:
				lastFailure = ""
				if text := strings.TrimSpace(piText(msg["content"])); text != "" {
					fmt.Fprintf(w, "\n%s\n", text)
					res.Text = text
				}
			}
		case "auto_retry_start":
			fmt.Fprintf(w, "retrying (attempt %v/%v): %s\n", event["attempt"], event["maxAttempts"], str(event["errorMessage"]))
		case "auto_retry_end":
			if v, ok := event["success"].(bool); ok && !v {
				lastFailure = or(str(event["finalError"]), "retries exhausted")
				fmt.Fprintf(w, "error: %s\n", lastFailure)
			}
		case "compaction_end":
			if m := str(event["errorMessage"]); m != "" {
				fmt.Fprintf(w, "compaction failed: %s\n", m)
			}
		case "extension_error":
			fmt.Fprintf(w, "extension error: %s\n", str(event["error"]))
		case "agent_end":
			agentEnded = !boolish(event["willRetry"])
		case "agent_settled":
			settled = true
			d.clearTools()
			if boolish(event["aborted"]) {
				res.Err = fmt.Errorf("%w: pi run aborted", ErrAgent)
			} else if lastFailure != "" {
				res.Err = fmt.Errorf("%w: %s", ErrAgent, lastFailure)
			} else {
				res.Err = nil
			}
			if !res.Completed {
				if res.Err != nil {
					fmt.Fprintln(w, "pi agent failed")
				} else {
					fmt.Fprintln(w, "pi agent completed")
				}
				res.Completed = true
			}
			if d.OnComplete != nil {
				d.OnComplete()
			}
		}
	}
	if readErr != nil && res.Err == nil && !settled {
		res.Err = readErr
	}
	if settled {
		return res
	}
	// Older Pi builds may exit after agent_end without agent_settled.
	if agentEnded && res.Err == nil {
		if lastFailure != "" {
			res.Err = fmt.Errorf("%w: %s", ErrAgent, lastFailure)
			fmt.Fprintln(w, "pi agent failed")
		} else {
			fmt.Fprintln(w, "pi agent completed")
		}
		res.Completed = true
		return res
	}
	if res.Err != nil {
		return res
	}
	reason := "no stream events received from pi agent"
	if res.Started {
		reason = "stream ended after " + or(lastEvent, "unknown event")
	}
	if lastFailure != "" {
		reason += ": " + lastFailure
	}
	fmt.Fprintf(w, "pi agent incomplete: %s\n", reason)
	res.Err = fmt.Errorf("%w: %s", ErrIncomplete, reason)
	return res
}

// piMaxLine bounds the memory used for one JSONL record. Larger records are
// discarded rather than ending the stream.
const piMaxLine = 8 * 1024 * 1024

type piLineReader struct {
	r   *bufio.Reader
	max int
}

// next returns the next LF-terminated record without the trailing CR/LF.
// skipped is true when the record exceeded max and was discarded.
func (l *piLineReader) next() (line []byte, skipped bool, err error) {
	var buf []byte
	for {
		chunk, err := l.r.ReadSlice('\n')
		if !skipped {
			if len(buf)+len(chunk) > l.max {
				skipped = true
				buf = nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			if err == io.EOF && (len(buf) > 0 || skipped) {
				return trimEOL(buf), skipped, nil
			}
			return nil, false, err
		}
		return trimEOL(buf), skipped, nil
	}
}

func trimEOL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// piText joins the text blocks of an assistant message, skipping thinking
// and tool-call blocks.
func piText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			block, _ := item.(map[string]any)
			if block == nil || str(block["type"]) != "text" {
				continue
			}
			if t := str(block["text"]); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func piToolLabel(name string, args any) string {
	name = or(name, "tool")
	a, _ := args.(map[string]any)
	if a == nil {
		return name
	}
	switch name {
	case "bash", "powershell":
		if cmd := str(a["command"]); cmd != "" {
			return "shell: `" + truncate(oneLine(cmd), 200) + "`"
		}
	}
	for _, key := range []string{"path", "file_path", "filePath", "pattern", "query"} {
		if s := str(a[key]); s != "" {
			return name + " " + truncate(oneLine(s), 200)
		}
	}
	return name
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
