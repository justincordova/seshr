package editor

import (
	"errors"

	"github.com/gofrs/flock"
)

var ErrLocked = errors.New("session file is locked by another process")

type Lock struct{ fl *flock.Flock }

func TryLock(path string) (*Lock, error) {
	fl := flock.New(path + ".lock")
	ok, err := fl.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrLocked
	}
	return &Lock{fl: fl}, nil
}

func (l *Lock) Release() error {
	if l == nil || l.fl == nil {
		return nil
	}
	return l.fl.Unlock()
}
