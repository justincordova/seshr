package claude

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/justincordova/seshr/internal/session"
)

// maxLineSize bounds a single JSONL record. Matches the Parse() scanner.
const maxLineSize = 10 * 1024 * 1024

// parseJSONLStream reads JSONL records from r, decodes each as a session.Turn,
// and returns the parsed turns and total bytes consumed.
//
// IMPORTANT: only newline-terminated records are consumed. A partial
// trailing line (the live agent mid-write) is left in the stream so the
// next incremental tick can re-read it from the start once the newline
// has been written. Counting bytes from terminated records only keeps
// cursor.ByteOffset aligned to a record boundary.
func parseJSONLStream(r io.Reader) ([]session.Turn, int64, error) {
	br := bufio.NewReaderSize(r, 64*1024)

	var turns []session.Turn
	var bytesRead int64
	lineNum := 0

	for {
		line, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// Spillover read for an oversized line. We still need the
			// full record to parse; switch to ReadBytes for the rest.
			rest, err2 := br.ReadBytes('\n')
			if err2 != nil {
				if errors.Is(err2, io.EOF) {
					// Partial oversized line at EOF — leave for next tick.
					return turns, bytesRead, nil
				}
				return turns, bytesRead, fmt.Errorf("read stream: %w", err2)
			}
			full := make([]byte, 0, len(line)+len(rest))
			full = append(full, line...)
			full = append(full, rest...)
			if len(full) > maxLineSize {
				return turns, bytesRead, fmt.Errorf("line %d exceeds max size (%d > %d)",
					lineNum, len(full), maxLineSize)
			}
			line = full
		} else if err != nil {
			if errors.Is(err, io.EOF) {
				// Partial trailing line. Do NOT count its bytes — let the
				// next tick re-read from this position once the writer
				// closes the record with a newline.
				return turns, bytesRead, nil
			}
			return turns, bytesRead, fmt.Errorf("read stream: %w", err)
		}
		// line ends in '\n' here.
		bytesRead += int64(len(line))
		content := bytes.TrimRight(line, "\n")
		if len(content) == 0 {
			lineNum++
			continue
		}
		turn, ok := parseLine(content, lineNum)
		if ok {
			turns = append(turns, turn)
		}
		lineNum++
	}
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
