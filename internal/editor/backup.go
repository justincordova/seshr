package editor

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
)

var ErrNoBackup = errors.New("no backup file present")

// CreateBackup snapshots path to path+".bak". The copy is staged to a
// sibling tmp file and renamed into place so a failure mid-copy (e.g. disk
// full) can never leave a truncated .bak — the .bak is the only safety net
// for destructive edits, and a later restore would trust it blindly.
func CreateBackup(path string) error {
	bak := path + ".bak"
	tmp := bak + ".tmp"
	if err := copyFile(path, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, bak); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename backup into place: %w", err)
	}
	return nil
}

func AtomicReplace(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return fmt.Errorf("rename: %w", err)
	}
	// Cross-device fallback. copyFile truncates its destination in place, so
	// copying straight onto dst would leave the real file half-written if the
	// copy failed midway. Stage onto a sibling of dst (same filesystem) and
	// rename into place so dst is only ever swapped atomically.
	tmpDst := dst + ".repl"
	if err := copyFile(src, tmpDst); err != nil {
		_ = os.Remove(tmpDst)
		return err
	}
	if err := os.Rename(tmpDst, dst); err != nil {
		_ = os.Remove(tmpDst)
		return fmt.Errorf("rename staged copy: %w", err)
	}
	if err := os.Remove(src); err != nil {
		// dst has already been swapped — the replace succeeded. Returning an
		// error here would make the caller believe the prune failed and
		// retry, and the retry's CreateBackup would overwrite the .bak with
		// already-pruned content. The leftover staging file is harmless: it
		// is truncated by the next run.
		slog.Warn("leftover staging file after replace", "path", src, "err", err)
	}
	return nil
}

// Restore copies path+".bak" back over path. It takes the per-session flock
// so a restore cannot interleave with a concurrent prune, and stages the
// copy before an atomic swap so a failure mid-copy never leaves the live
// file truncated.
func Restore(path string) error {
	bak := path + ".bak"
	if _, err := os.Stat(bak); err != nil {
		if os.IsNotExist(err) {
			return ErrNoBackup
		}
		return err
	}
	lock, err := TryLock(path)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	tmp := path + ".restore.tmp"
	if err := copyFile(bak, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := AtomicReplace(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func copyFile(src, dst string) (retErr error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() {
		if err := in.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close src: %w", err)
		}
	}()
	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	// Propagate the source's permissions: session files may be 0600 and
	// their backups/replacements must not silently widen read access.
	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() {
		if err := out.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close dst: %w", err)
		}
	}()
	// O_CREATE perms only apply to newly created files; if dst already
	// existed (stale staging file), align it explicitly.
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	return nil
}
