package claude

import (
	"bufio"
	"encoding/json"
	"io"
	"sort"
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
	// Claude JSONL nests role/content under "message":
	//   {"type":"assistant","message":{"role":"assistant","content":[...]}}
	type record struct {
		Type    string `json:"type"`
		Message struct {
			Content []contentBlock `json:"content"`
		} `json:"message"`
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
		if rec.Type != "assistant" {
			continue
		}
		for _, blk := range rec.Message.Content {
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
		// Fall back to the first key alphabetically. Keys must be sorted —
		// map iteration order is random, and a nondeterministic label would
		// flicker in the picker between refreshes.
		if firstArg == "" {
			keys := make([]string, 0, len(args))
			for k := range args {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				var s string
				if err := json.Unmarshal(args[k], &s); err == nil && s != "" {
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
	// Rune-aware truncation. label is built from user-controlled fields
	// (filenames, commands, URLs) that may contain multibyte characters.
	// Byte slicing would split a rune mid-encoding and emit invalid UTF-8
	// into the picker's live-task column.
	runes := []rune(label)
	if len(runes) > 30 {
		label = string(runes[:30])
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
