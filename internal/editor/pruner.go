package editor

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/justincordova/seshr/internal/parser"
)

func Prune(sess *parser.Session, selection Selection, dstPath string) error {
	if dstPath == sess.Path {
		return fmt.Errorf("destination must differ from source (%s)", dstPath)
	}
	pruned := map[int]bool{}
	for idx := range selection.Turns {
		if idx < 0 || idx >= len(sess.Turns) {
			continue
		}
		pruned[sess.Turns[idx].RawIndex] = true
		for _, extra := range sess.Turns[idx].ExtraLineIndices {
			pruned[extra] = true
		}
	}

	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", dstPath, err)
	}
	defer func() { _ = f.Close() }()

	src, err := os.Open(sess.Path)
	if err != nil {
		return fmt.Errorf("open source %s: %w", sess.Path, err)
	}
	defer func() { _ = src.Close() }()

	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	lineIdx := 0
	w := bufio.NewWriter(f)

	for scanner.Scan() {
		if !pruned[lineIdx] {
			if _, err := w.Write(scanner.Bytes()); err != nil {
				return fmt.Errorf("write line %d: %w", lineIdx, err)
			}
			if err := w.WriteByte('\n'); err != nil {
				return fmt.Errorf("write newline %d: %w", lineIdx, err)
			}
		}
		lineIdx++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan source: %w", err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	return nil
}

func PruneSession(sess *parser.Session, selection Selection) error {
	path := sess.Path
	lock, err := TryLock(path)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	if err := CreateBackup(path); err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	tmp := path + ".tmp"
	if err := Prune(sess, selection, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write tmp: %w", err)
	}

	p := parser.NewClaude()
	if _, perr := p.Parse(context.Background(), tmp); perr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("validate pruned file: %w", perr)
	}

	if err := AtomicReplace(tmp, path); err != nil {
		return fmt.Errorf("atomic replace: %w", err)
	}

	slog.Info("pruned session",
		"path", path,
		"removed_turns", len(selection.Turns),
	)
	return nil
}
