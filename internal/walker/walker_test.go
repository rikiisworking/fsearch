package walker

import (
	"context"
	"errors"
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

	got, err := collectWalk(context.Background(), root, ignore.New([]string{"go", "md"}, nil))
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	sort.Strings(got)
	want := []string{
		filepath.Join(root, "a.go"),
		filepath.Join(root, "b.md"),
		filepath.Join(root, "sub", "d.go"),
	}
	sort.Strings(want)
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWalkMissingRoot(t *testing.T) {
	files := make(chan string, 1)
	err := Walk(context.Background(), filepath.Join(t.TempDir(), "nope"), ignore.New(nil, nil), files, nil)
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestWalkNilFilter(t *testing.T) {
	files := make(chan string, 1)
	err := Walk(context.Background(), t.TempDir(), nil, files, nil)
	if err == nil {
		t.Fatal("expected error for nil filter")
	}
}

func TestWalkSingleFileRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "only.go")
	mustWrite(t, path, "package only")

	// Allowed by filter: emit the file.
	got, err := collectWalk(context.Background(), path, ignore.New([]string{"go"}, nil))
	if err != nil {
		t.Fatalf("Walk allowed: %v", err)
	}
	if len(got) != 1 || got[0] != path {
		t.Fatalf("allowed: got %v, want [%q]", got, path)
	}

	// Rejected by extension filter: emit nothing.
	got, err = collectWalk(context.Background(), path, ignore.New([]string{"md"}, nil))
	if err != nil {
		t.Fatalf("Walk rejected: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("rejected: got %v, want none", got)
	}
}

func TestWalkSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	realFile := filepath.Join(root, "real.go")
	mustWrite(t, realFile, "package real")

	// File symlink should not be emitted.
	linkFile := filepath.Join(root, "link.go")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Fatalf("Symlink file: %v", err)
	}

	// Directory symlink should not be entered.
	realDir := filepath.Join(root, "realdir")
	mustWrite(t, filepath.Join(realDir, "hidden.go"), "package hidden")
	linkDir := filepath.Join(root, "linkdir")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("Symlink dir: %v", err)
	}

	got, err := collectWalk(context.Background(), root, ignore.New([]string{"go"}, nil))
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	// Expect only the real files, not the symlink paths.
	want := []string{
		realFile,
		filepath.Join(realDir, "hidden.go"),
	}
	sort.Strings(got)
	sort.Strings(want)
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWalkContextCancel(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before walk starts

	files := make(chan string, 8)
	err := Walk(ctx, root, ignore.New(nil, nil), files, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Walk error = %v, want context.Canceled", err)
	}
}

func TestWalkContextCancelEmptyTree(t *testing.T) {
	// No files emitted: cancel must still be reported (not nil success).
	root := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	files := make(chan string, 8)
	err := Walk(ctx, root, ignore.New(nil, nil), files, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Walk empty cancelled = %v, want context.Canceled", err)
	}
}

func TestWalkContextCancelAllFiltered(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Extension filter matches nothing → no emit path.
	files := make(chan string, 8)
	err := Walk(ctx, root, ignore.New([]string{"md"}, nil), files, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Walk filtered cancelled = %v, want context.Canceled", err)
	}
}

func TestWalkUnreadableDir(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.go")
	mustWrite(t, good, "package good")

	badDir := filepath.Join(root, "locked")
	mustWrite(t, filepath.Join(badDir, "secret.go"), "package secret")
	if err := os.Chmod(badDir, 0); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(badDir, 0o755) })

	// If still readable (e.g. root), the walkErr path won't fire.
	if f, err := os.Open(badDir); err == nil {
		f.Close()
		t.Skip("unreadable dir still openable (root?); skip walkErr test")
	}

	got, err := collectWalk(context.Background(), root, ignore.New([]string{"go"}, nil))
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	// Sibling file still found; contents of locked/ are not.
	if len(got) != 1 || got[0] != good {
		t.Fatalf("got %v, want [%q]", got, good)
	}
}

func TestWalkOnErrorUnreadableDir(t *testing.T) {
	// walkErr path must invoke onError while still completing the walk.
	root := t.TempDir()
	good := filepath.Join(root, "good.go")
	mustWrite(t, good, "package good")

	badDir := filepath.Join(root, "locked")
	mustWrite(t, filepath.Join(badDir, "secret.go"), "package secret")
	if err := os.Chmod(badDir, 0); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(badDir, 0o755) })

	if f, err := os.Open(badDir); err == nil {
		f.Close()
		t.Skip("unreadable dir still openable (root?); skip onError test")
	}

	var errPaths []string
	onError := func(path string, err error) {
		if err == nil {
			t.Errorf("onError called with nil err for %q", path)
		}
		errPaths = append(errPaths, path)
	}

	files := make(chan string, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Walk(context.Background(), root, ignore.New([]string{"go"}, nil), files, onError)
		close(files)
	}()

	var got []string
	for f := range files {
		got = append(got, f)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Walk: %v", err)
	}

	if len(got) != 1 || got[0] != good {
		t.Fatalf("paths = %v, want [%q]", got, good)
	}
	// WalkDir typically reports the locked directory itself.
	found := false
	for _, p := range errPaths {
		if p == badDir || filepath.Base(p) == "locked" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("onError paths = %v, want one containing locked dir %q", errPaths, badDir)
	}
}

func TestWalkRootNotPrunedByBasename(t *testing.T) {
	// Default skip includes "bin"; walking a root named "bin" must still enter it.
	root := filepath.Join(t.TempDir(), "bin")
	mustWrite(t, filepath.Join(root, "main.go"), "package main")

	got, err := collectWalk(context.Background(), root, ignore.New([]string{"go"}, nil))
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	want := filepath.Join(root, "main.go")
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %v, want [%q]", got, want)
	}
}

// collectWalk runs Walk and returns collected absolute paths.
func collectWalk(ctx context.Context, root string, filter Filter) ([]string, error) {
	files := make(chan string, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Walk(ctx, root, filter, files, nil)
		close(files)
	}()

	var got []string
	for f := range files {
		got = append(got, f)
	}
	return got, <-errCh
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
