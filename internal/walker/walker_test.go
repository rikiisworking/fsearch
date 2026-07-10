package walker

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/nick/fsearch/internal/ignore"
)

func TestWalk(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a")
	mustWrite(t, filepath.Join(root, "b.md"), "# b")
	mustWrite(t, filepath.Join(root, "c.py"), "print(1)")
	mustWrite(t, filepath.Join(root, "sub", "d.go"), "package d")
	mustWrite(t, filepath.Join(root, ".git", "config"), "git")
	mustWrite(t, filepath.Join(root, "node_modules", "pkg", "x.js"), "x")

	filter := ignore.New([]string{"go", "md"}, nil)
	files := make(chan string, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Walk(context.Background(), root, filter, files)
		close(files)
	}()

	var got []string
	for f := range files {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			t.Errorf("Rel(%q): %v", f, err)
			continue
		}
		got = append(got, rel)
	}
	if err := <-errCh; err != nil {
		t.Errorf("Walk: %v", err)
	}

	sort.Strings(got)
	want := []string{"a.go", "b.md", filepath.Join("sub", "d.go")}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			return
		}
	}
}

func TestWalkMissingRoot(t *testing.T) {
	files := make(chan string, 1)
	err := Walk(context.Background(), filepath.Join(t.TempDir(), "nope"), ignore.New(nil, nil), files)
	if err == nil {
		t.Errorf("expected error for missing root")
	}
}

func TestWalkNilFilter(t *testing.T) {
	files := make(chan string, 1)
	err := Walk(context.Background(), t.TempDir(), nil, files)
	if err == nil {
		t.Errorf("expected error for nil filter")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Errorf("MkdirAll: %v", err)
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Errorf("WriteFile: %v", err)
	}
}
