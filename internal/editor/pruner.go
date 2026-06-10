package editor

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"

	"github.com/justincordova/seshr/internal/session"
)

func Prune(sess *session.Session, selection Selection, dstPath string) (retErr error) {
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
	w := bufio.NewWriter(f)
	// Order matters: flush buffered writes → fsync → close. On any error
	// path the deferred chain captures the first failure into retErr so a
	// silent truncation cannot masquerade as success.
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close %s: %w", dstPath, err)
		}
	}()
	defer func() {
		if err := f.Sync(); err != nil && retErr == nil {
			retErr = fmt.Errorf("sync %s: %w", dstPath, err)
		}
	}()
	defer func() {
		if err := w.Flush(); err != nil && retErr == nil {
			retErr = fmt.Errorf("flush %s: %w", dstPath, err)
		}
	}()

	src, err := os.Open(sess.Path)
	if err != nil {
		return fmt.Errorf("open source %s: %w", sess.Path, err)
	}
	defer func() { _ = src.Close() }()

	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	lineIdx := 0

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
	return nil
}

func PruneSession(sess *session.Session, selection Selection) error {
	// An empty selection is a no-op: rewriting would change nothing but
	// CreateBackup below would still overwrite the .bak — destroying the
	// only copy of the pre-previous-prune state.
	if len(selection.Turns) == 0 {
		return nil
	}

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

	if err := AtomicReplace(tmp, path); err != nil {
		return fmt.Errorf("atomic replace: %w", err)
	}

	slog.Info("pruned session",
		"path", path,
		"removed_turns", len(selection.Turns),
	)
	return nil
}
