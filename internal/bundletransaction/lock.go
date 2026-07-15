// Package bundletransaction owns the one repository-local lock that serializes
// complete bundle observations and replacement transactions.
package bundletransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const retryInterval = 5 * time.Millisecond

type Guard struct {
	file *os.File
}

func LockPath(repositoryRoot string) string {
	return filepath.Clean(repositoryRoot)
}

func Acquire(ctx context.Context, repositoryRoot string) (*Guard, error) {
	if ctx == nil {
		return nil, errors.New("bundle transaction lock requires a context")
	}
	info, err := os.Stat(repositoryRoot)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("bundle transaction repository root is unavailable: %w", err)
	}
	file, err := os.Open(LockPath(repositoryRoot))
	if err != nil {
		return nil, fmt.Errorf("open bundle transaction repository directory: %w", err)
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &Guard{file: file}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			file.Close()
			return nil, fmt.Errorf("acquire bundle transaction lock: %w", err)
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, fmt.Errorf("acquire bundle transaction lock: %w", ctx.Err())
		case <-time.After(retryInterval):
		}
	}
}

func (guard *Guard) Release() error {
	if guard == nil || guard.file == nil {
		return nil
	}
	file := guard.file
	guard.file = nil
	unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	closeErr := file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release bundle transaction lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close bundle transaction lock: %w", closeErr)
	}
	return nil
}

func WithExclusive(ctx context.Context, repositoryRoot string, observe func() error) error {
	guard, err := Acquire(ctx, repositoryRoot)
	if err != nil {
		return err
	}
	defer guard.Release()
	return observe()
}
