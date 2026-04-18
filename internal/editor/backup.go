package editor

import (
	"errors"
	"fmt"
	"io"
	"os"
)

var ErrNoBackup = errors.New("no backup file present")

func CreateBackup(path string) error {
	return copyFile(path, path+".bak")
}

func AtomicReplace(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return fmt.Errorf("rename: %w", err)
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func Restore(path string) error {
	bak := path + ".bak"
	if _, err := os.Stat(bak); err != nil {
		if os.IsNotExist(err) {
			return ErrNoBackup
		}
		return err
	}
	return copyFile(bak, path)
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
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() {
		if err := out.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close dst: %w", err)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	return nil
}
