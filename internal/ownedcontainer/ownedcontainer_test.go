package ownedcontainer_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/matty/internal/ownedcontainer"
)

func TestProvisionProvesOnlyAtomicCreations(t *testing.T) {
	root := t.TempDir()
	preexisting := filepath.Join(root, "preexisting")
	created := filepath.Join(root, "created")
	if err := os.Mkdir(preexisting, 0o700); err != nil {
		t.Fatal(err)
	}
	records, err := ownedcontainer.Provision([]ownedcontainer.Record{{Path: preexisting, Kind: ownedcontainer.Directory}, {Path: created, Kind: ownedcontainer.Directory}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Path != created || records[0].Kind != ownedcontainer.Directory {
		t.Fatalf("Created() = %#v, want only %s", records, created)
	}
}

func TestCleanupPreservesNonemptyAndRemovesExactEmptyOwnedPaths(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	nonempty := filepath.Join(root, "nonempty")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nonempty, []byte("unmanaged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := ownedcontainer.Preview([]ownedcontainer.Record{{Path: empty, Kind: ownedcontainer.Directory}, {Path: nonempty, Kind: ownedcontainer.File}})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Verify(); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Fatalf("empty owned directory was not removed: %v", err)
	}
	data, err := os.ReadFile(nonempty)
	if err != nil || string(data) != "unmanaged\n" {
		t.Fatalf("nonempty file changed: %q, %v", data, err)
	}
}

func TestVerifyRejectsChangeAfterPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := ownedcontainer.Preview([]ownedcontainer.Record{{Path: path, Kind: ownedcontainer.File}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("concurrent"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := plan.Verify(); !errors.Is(err, ownedcontainer.ErrStalePlan) {
		t.Fatalf("Verify() error = %v, want ErrStalePlan", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "concurrent" {
		t.Fatalf("stale verification mutated file: %q", data)
	}
}
