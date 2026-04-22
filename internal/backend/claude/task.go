package claude

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// extractLastToolUse reads JSONL records from r and returns the last tool_use
// name + first argument as a string ≤ 30 chars.
func extractLastToolUse(r io.Reader) string {
	type contentBlock struct {
		Type  string          `json:"type"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	type record struct {
		Role    string         `json:"role"`
		Content []contentBlock `json:"content"`
	}

	var lastTask string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Role != "assistant" {
			continue
		}
		for _, blk := range rec.Content {
			if blk.Type != "tool_use" {
				continue
			}
			task := formatToolTask(blk.Name, blk.Input)
			if task != "" {
				lastTask = task
			}
		}
	}
	return lastTask
}

// formatToolTask formats a tool name and its first input argument as a
// ≤ 30-char task label.
func formatToolTask(name string, input json.RawMessage) string {
	if name == "" {
		return ""
	}
	// Try to extract the first string argument from the input object.
	var args map[string]json.RawMessage
	firstArg := ""
	if err := json.Unmarshal(input, &args); err == nil {
		// Priority order of common path args.
		for _, key := range []string{"file_path", "path", "command", "url"} {
			if raw, ok := args[key]; ok {
				var s string
				if err := json.Unmarshal(raw, &s); err == nil && s != "" {
					firstArg = lastPathComponent(s)
					break
				}
			}
		}
		// Fall back to the first key alphabetically.
		if firstArg == "" {
			for _, raw := range args {
				var s string
				if err := json.Unmarshal(raw, &s); err == nil && s != "" {
					firstArg = s
					break
				}
			}
		}
	}

	var label string
	if firstArg != "" {
		label = name + " " + firstArg
	} else {
		label = name
	}
	if len(label) > 30 {
		label = label[:30]
	}
	return label
}

// lastPathComponent extracts the last segment of a path-like string.
func lastPathComponent(s string) string {
	parts := strings.Split(s, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return s
}
