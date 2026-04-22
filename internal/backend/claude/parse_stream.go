package claude

import (
	"bufio"
	"fmt"
	"io"

	"github.com/justincordova/seshr/internal/session"
)

// parseJSONLStream reads JSONL records from r, decodes each as a session.Turn,
// and returns the parsed turns and total bytes consumed.
func parseJSONLStream(r io.Reader) ([]session.Turn, int64, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var turns []session.Turn
	var bytesRead int64
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		bytesRead += int64(len(line)) + 1 // +1 for the newline
		if len(line) == 0 {
			lineNum++
			continue
		}
		turn, ok := parseLine(line, lineNum)
		if ok {
			turns = append(turns, turn)
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return nil, bytesRead, fmt.Errorf("scan stream: %w", err)
	}
	return turns, bytesRead, nil
}

// parseJSONLRange reads from r and returns the turns with indices in [from, to).
func parseJSONLRange(r io.Reader, from, to int) ([]session.Turn, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var turns []session.Turn
	turnIdx := 0
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			lineNum++
			continue
		}
		turn, ok := parseLine(line, lineNum)
		if !ok {
			lineNum++
			continue
		}
		if turnIdx >= from && turnIdx < to {
			turns = append(turns, turn)
		}
		if turnIdx >= to {
			break
		}
		turnIdx++
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan range: %w", err)
	}
	return turns, nil
}
