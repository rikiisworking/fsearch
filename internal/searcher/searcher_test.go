package searcher

import (
	"context"
	"os"
	"path/filepath"
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

	matches, err := SearchFile(context.Background(), path, "TODO")
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
}

func TestSearchFileNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Errorf("WriteFile: %v", err)
		return
	}

	matches, err := SearchFile(context.Background(), path, "TODO")
	if err != nil {
		t.Errorf("SearchFile: %v", err)
		return
	}
	if len(matches) != 0 {
		t.Errorf("got %v, want none", matches)
	}
}

func TestSearchFileEmptyKeyword(t *testing.T) {
	_, err := SearchFile(context.Background(), "x", "")
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

	matches, err := SearchFile(context.Background(), path, "hello")
	if err != nil {
		t.Errorf("SearchFile: %v", err)
		return
	}
	if matches != nil {
		t.Errorf("binary should be skipped, got %v", matches)
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
