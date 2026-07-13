package searcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSearchFileHits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	content := "package main\n\n// TODO: first\nfunc main() {}\n// TODO: second\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Errorf("WriteFile: %v", err)
		return
	}

	matches, err := SearchFile(context.Background(), path, FileOptions{Keyword: "TODO"})
	if err != nil {
		t.Errorf("SearchFile: %v", err)
		return
	}
	if len(matches) != 2 {
		t.Errorf("got %d matches, want 2: %+v", len(matches), matches)
		return
	}
	if matches[0].Line != 3 || matches[1].Line != 5 {
		t.Errorf("lines = %d,%d want 3,5", matches[0].Line, matches[1].Line)
	}
	if matches[0].Path != path {
		t.Errorf("path = %q, want %q", matches[0].Path, path)
	}
	if matches[0].Content != "// TODO: first" {
		t.Errorf("content = %q", matches[0].Content)
	}
	if matches[0].Before != nil || matches[0].After != nil {
		t.Errorf("context slices should be empty for now: %+v", matches[0])
	}
}

func TestSearchFileNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Errorf("WriteFile: %v", err)
		return
	}

	matches, err := SearchFile(context.Background(), path, FileOptions{Keyword: "TODO"})
	if err != nil {
		t.Errorf("SearchFile: %v", err)
		return
	}
	if len(matches) != 0 {
		t.Errorf("got %v, want none", matches)
	}
}

func TestSearchFileEmptyKeyword(t *testing.T) {
	_, err := SearchFile(context.Background(), "x", FileOptions{Keyword: ""})
	if err == nil {
		t.Errorf("expected error for empty keyword")
	}
}

func TestSearchFileBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin.dat")
	data := []byte("hello\x00world")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Errorf("WriteFile: %v", err)
		return
	}

	matches, err := SearchFile(context.Background(), path, FileOptions{Keyword: "hello"})
	if err != nil {
		t.Errorf("SearchFile: %v", err)
		return
	}
	if matches != nil {
		t.Errorf("binary should be skipped, got %v", matches)
	}
}

func TestSearchFileIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("todo one\nTODO two\nToDo three\nnope\n"), 0o644); err != nil {
		t.Errorf("WriteFile: %v", err)
		return
	}

	// Case-sensitive (default): only exact "TODO"
	got, err := SearchFile(context.Background(), path, FileOptions{Keyword: "TODO"})
	if err != nil {
		t.Errorf("SearchFile: %v", err)
		return
	}
	if len(got) != 1 || got[0].Line != 2 {
		t.Errorf("case-sensitive: got %+v, want 1 hit on line 2", got)
	}

	// Ignore case: all three variants
	got, err = SearchFile(context.Background(), path, FileOptions{
		Keyword:    "TODO",
		IgnoreCase: true,
	})
	if err != nil {
		t.Errorf("SearchFile: %v", err)
		return
	}
	if len(got) != 3 {
		t.Errorf("ignore-case: got %d hits, want 3: %+v", len(got), got)
	}
}

func TestSearch(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n// TODO here\n")
	mustWrite(t, filepath.Join(root, "b.md"), "# no\n")
	mustWrite(t, filepath.Join(root, "sub", "c.go"), "TODO again\n")
	mustWrite(t, filepath.Join(root, ".git", "x"), "TODO hidden\n")

	var got []Match
	err := Search(context.Background(), Options{
		Root:        root,
		Keyword:     "TODO",
		AllowedExts: []string{"go"},
		Workers:     2,
	}, func(m Match) error {
		got = append(got, m)
		return nil
	})
	if err != nil {
		t.Errorf("Search: %v", err)
		return
	}
	if len(got) != 2 {
		t.Errorf("got %d matches, want 2: %+v", len(got), got)
		return
	}
	for _, m := range got {
		if m.Line < 1 {
			t.Errorf("bad line: %+v", m)
		}
		if m.Path == "" {
			t.Errorf("empty path: %+v", m)
		}
	}
}

func TestSearchEmptyKeyword(t *testing.T) {
	err := Search(context.Background(), Options{Root: ".", Keyword: ""}, func(Match) error {
		return nil
	})
	if err == nil {
		t.Errorf("expected error for empty keyword")
	}
}

func TestSearchOnError(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.go")
	bad := filepath.Join(root, "bad.go")
	mustWrite(t, good, "package good\n// TODO ok\n")
	mustWrite(t, bad, "package bad\n// TODO hidden by perms\n")

	if err := os.Chmod(bad, 0); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	// Restore perms so TempDir cleanup can remove the file.
	t.Cleanup(func() { _ = os.Chmod(bad, 0o644) })

	// If we can still open it (e.g. running as root), OnError won't fire.
	if f, err := os.Open(bad); err == nil {
		f.Close()
		t.Skip("unreadable file still openable (root?); skip OnError test")
	}

	var (
		mu         sync.Mutex
		errPaths   []string
		hitContent []string
	)
	err := Search(context.Background(), Options{
		Root:    root,
		Keyword: "TODO",
		Workers: 2,
		OnError: func(path string, err error) {
			mu.Lock()
			errPaths = append(errPaths, path)
			mu.Unlock()
		},
	}, func(m Match) error {
		mu.Lock()
		hitContent = append(hitContent, m.Content)
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(errPaths) != 1 || errPaths[0] != bad {
		t.Errorf("OnError paths = %v, want [%q]", errPaths, bad)
	}
	if len(hitContent) != 1 || hitContent[0] != "// TODO ok" {
		t.Errorf("hits = %v, want one good hit", hitContent)
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
