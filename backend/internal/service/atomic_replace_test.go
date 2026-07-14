package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicReplaceFileReplacesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(source, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := atomicReplaceFile(source, target); err != nil {
		t.Fatalf("replace target: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("target content = %q, want new", data)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source should be moved after replacement: %v", err)
	}
}
