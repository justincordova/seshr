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
// and returns the parsed turns, any compact boundaries within the stream, and
// the total bytes consumed.
//
// Boundary TurnIndex values are relative to this stream (first turn = 0), to
// match the incremental contract where the caller appends the returned turns
// to its existing window.
//
// IMPORTANT: only newline-terminated records are consumed. A partial
// trailing line (the live agent mid-write) is left in the stream so the
// next incremental tick can re-read it from the start once the newline
// has been written. Counting bytes from terminated records only keeps
// cursor.ByteOffset aligned to a record boundary.
func parseJSONLStream(r io.Reader) ([]session.Turn, []session.CompactBoundary, int64, error) {
	br := bufio.NewReaderSize(r, 64*1024)

	var turns []session.Turn
	var boundaries []session.CompactBoundary
	var bytesRead int64
	lineNum := 0

	for {
		line, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// Spillover read for an oversized line. We still need the full
			// record to parse; switch to ReadBytes for the rest. ReadSlice
			// returns a slice into the internal buffer that is invalidated by
			// the next read, so copy the prefix out BEFORE calling ReadBytes.
			prefix := make([]byte, len(line))
			copy(prefix, line)
			rest, err2 := br.ReadBytes('\n')
			if err2 != nil {
				if errors.Is(err2, io.EOF) {
					// Partial oversized line at EOF — leave for next tick.
					return turns, boundaries, bytesRead, nil
				}
				return turns, boundaries, bytesRead, fmt.Errorf("read stream: %w", err2)
			}
			full := make([]byte, 0, len(prefix)+len(rest))
			full = append(full, prefix...)
			full = append(full, rest...)
			if len(full) > maxLineSize {
				return turns, boundaries, bytesRead, fmt.Errorf("line %d exceeds max size (%d > %d)",
					lineNum, len(full), maxLineSize)
			}
			line = full
		} else if err != nil {
			if errors.Is(err, io.EOF) {
				// Partial trailing line. Do NOT count its bytes — let the
				// next tick re-read from this position once the writer
				// closes the record with a newline.
				return turns, boundaries, bytesRead, nil
			}
			return turns, boundaries, bytesRead, fmt.Errorf("read stream: %w", err)
		}
		// line ends in '\n' here.
		bytesRead += int64(len(line))
		content := bytes.TrimRight(line, "\n")
		if len(content) == 0 {
			lineNum++
			continue
		}
		// Detect compact_boundary system records before the normal turn parse
		// path (parseLine drops all system records). TurnIndex is stream-
		// relative: the boundary sits before the next turn we append.
		if cb, ok := parseCompactBoundary(content, len(turns)); ok {
			boundaries = append(boundaries, cb)
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
