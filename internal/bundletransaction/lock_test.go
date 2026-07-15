package bundletransaction

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestExclusiveSerializesRepositoryBundleObservers(t *testing.T) {
	repository := t.TempDir()
	first, err := Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan *Guard, 1)
	errs := make(chan error, 1)
	go func() {
		guard, err := Acquire(context.Background(), repository)
		if err != nil {
			errs <- err
			return
		}
		acquired <- guard
	}()
	select {
	case <-acquired:
		t.Fatal("second observer acquired the bundle transaction lock concurrently")
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case second := <-acquired:
		if err := second.Release(); err != nil {
			t.Fatal(err)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("waiting observer did not acquire released bundle transaction lock")
	}
	if got := LockPath(repository); got != filepath.Clean(repository) {
		t.Fatalf("LockPath() = %s", got)
	}
}

func TestAcquireHonorsCancellationWhileWaiting(t *testing.T) {
	repository := t.TempDir()
	guard, err := Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := Acquire(ctx, repository); err == nil {
		t.Fatal("cancelled waiter unexpectedly acquired the lock")
	}
}
