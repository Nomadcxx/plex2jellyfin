package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectRoot_FromCWD(t *testing.T) {
	root := makeTestProjectRoot(t)
	got, err := resolveProjectRoot(root, "")
	if err != nil {
		t.Fatalf("resolveProjectRoot error: %v", err)
	}
	if got != root {
		t.Fatalf("expected %q, got %q", root, got)
	}
}

func TestResolveProjectRoot_FromExecutableParent(t *testing.T) {
	root := makeTestProjectRoot(t)
	execPath := filepath.Join(root, "build", "installer")
	got, err := resolveProjectRoot("/tmp/not-jw", execPath)
	if err != nil {
		t.Fatalf("resolveProjectRoot error: %v", err)
	}
	if got != root {
		t.Fatalf("expected %q, got %q", root, got)
	}
}

func TestResolveProjectRoot_ErrorWhenNotFound(t *testing.T) {
	_, err := resolveProjectRoot("/tmp", "/usr/local/bin/installer")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func makeTestProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mkdirAll(t, filepath.Join(root, "cmd", "jellywatch"))
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/jellywatch\n")
	writeFile(t, filepath.Join(root, "cmd", "jellywatch", "main.go"), "package main\nfunc main(){}\n")

	return root
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
