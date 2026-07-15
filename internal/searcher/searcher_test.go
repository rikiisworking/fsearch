package searcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sync/errgroup"
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
		t.Errorf("context slices should be empty when ContextLines=0: %+v", matches[0])
	}
}

func TestSearchFileContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	// lines: 1=alpha, 2=beta, 3=HIT one, 4=gamma, 5=HIT two, 6=delta
	content := "alpha\nbeta\nHIT one\ngamma\nHIT two\ndelta\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name    string
		n       int
		wantLen int
		check   func(t *testing.T, matches []Match)
	}{
		{
			name:    "N=0 empty context",
			n:       0,
			wantLen: 2,
			check: func(t *testing.T, matches []Match) {
				for _, m := range matches {
					if m.Before != nil || m.After != nil {
						t.Errorf("want nil context, got Before=%v After=%v", m.Before, m.After)
					}
				}
			},
		},
		{
			name:    "N=1 middle hits",
			n:       1,
			wantLen: 2,
			check: func(t *testing.T, matches []Match) {
				if matches[0].Line != 3 || matches[0].Content != "HIT one" {
					t.Errorf("match0 = %+v", matches[0])
				}
				if !equalStrings(matches[0].Before, []string{"beta"}) {
					t.Errorf("match0 Before = %v, want [beta]", matches[0].Before)
				}
				if !equalStrings(matches[0].After, []string{"gamma"}) {
					t.Errorf("match0 After = %v, want [gamma]", matches[0].After)
				}
				if matches[1].Line != 5 {
					t.Errorf("match1 line = %d", matches[1].Line)
				}
				if !equalStrings(matches[1].Before, []string{"gamma"}) {
					t.Errorf("match1 Before = %v, want [gamma]", matches[1].Before)
				}
				if !equalStrings(matches[1].After, []string{"delta"}) {
					t.Errorf("match1 After = %v, want [delta]", matches[1].After)
				}
			},
		},
		{
			name:    "N larger than file clamps",
			n:       100,
			wantLen: 2,
			check: func(t *testing.T, matches []Match) {
				if !equalStrings(matches[0].Before, []string{"alpha", "beta"}) {
					t.Errorf("match0 Before = %v", matches[0].Before)
				}
				if !equalStrings(matches[0].After, []string{"gamma", "HIT two", "delta"}) {
					t.Errorf("match0 After = %v", matches[0].After)
				}
				if !equalStrings(matches[1].Before, []string{"alpha", "beta", "HIT one", "gamma"}) {
					t.Errorf("match1 Before = %v", matches[1].Before)
				}
				if !equalStrings(matches[1].After, []string{"delta"}) {
					t.Errorf("match1 After = %v", matches[1].After)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := SearchFile(context.Background(), path, FileOptions{
				Keyword:      "HIT",
				ContextLines: tt.n,
			})
			if err != nil {
				t.Fatalf("SearchFile: %v", err)
			}
			if len(matches) != tt.wantLen {
				t.Fatalf("got %d matches, want %d: %+v", len(matches), tt.wantLen, matches)
			}
			tt.check(t, matches)
		})
	}
}

func TestSearchFileContextEdges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edges.txt")
	// HIT on first and last line
	if err := os.WriteFile(path, []byte("HIT first\nmiddle\nHIT last\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	matches, err := SearchFile(context.Background(), path, FileOptions{
		Keyword:      "HIT",
		ContextLines: 1,
	})
	if err != nil {
		t.Fatalf("SearchFile: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}

	// first line: empty Before, After = middle
	if matches[0].Line != 1 {
		t.Errorf("match0 line = %d", matches[0].Line)
	}
	if len(matches[0].Before) != 0 {
		t.Errorf("match0 Before = %v, want empty", matches[0].Before)
	}
	if !equalStrings(matches[0].After, []string{"middle"}) {
		t.Errorf("match0 After = %v", matches[0].After)
	}

	// last line: Before = middle, empty After
	if matches[1].Line != 3 {
		t.Errorf("match1 line = %d", matches[1].Line)
	}
	if !equalStrings(matches[1].Before, []string{"middle"}) {
		t.Errorf("match1 Before = %v", matches[1].Before)
	}
	if len(matches[1].After) != 0 {
		t.Errorf("match1 After = %v, want empty", matches[1].After)
	}
}

func TestSearchFileContextIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("prev\ntodo mid\nnext\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	matches, err := SearchFile(context.Background(), path, FileOptions{
		Keyword:      "TODO",
		IgnoreCase:   true,
		ContextLines: 1,
	})
	if err != nil {
		t.Fatalf("SearchFile: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if !equalStrings(matches[0].Before, []string{"prev"}) || !equalStrings(matches[0].After, []string{"next"}) {
		t.Errorf("context = Before=%v After=%v", matches[0].Before, matches[0].After)
	}
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

	got, err := collectSearch(context.Background(), Options{
		Root:        root,
		Keyword:     "TODO",
		AllowedExts: []string{"go"},
		Workers:     2,
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

func TestSearchGitignore(t *testing.T) {
	// Root .gitignore should hide matching paths from Search.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".gitignore"), "*.skip\nhidden/\n")
	mustWrite(t, filepath.Join(root, "keep.go"), "package keep\n// TODO visible\n")
	mustWrite(t, filepath.Join(root, "noise.skip"), "package noise\n// TODO hidden by gitignore\n")
	mustWrite(t, filepath.Join(root, "hidden", "x.go"), "package hidden\n// TODO in ignored dir\n")

	got, err := collectSearch(context.Background(), Options{
		Root:        root,
		Keyword:     "TODO",
		AllowedExts: []string{"go", "skip"}, // allow .skip ext so only gitignore hides it
		Workers:     2,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1 (only keep.go): %+v", len(got), got)
	}
	if !strings.Contains(got[0].Path, "keep.go") {
		t.Errorf("hit path = %q, want keep.go", got[0].Path)
	}
	if !strings.Contains(got[0].Content, "visible") {
		t.Errorf("content = %q, want visible hit", got[0].Content)
	}
}

func TestSearchNoGitignore(t *testing.T) {
	// NoGitignore should search paths that .gitignore would hide.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".gitignore"), "*.skip\n")
	mustWrite(t, filepath.Join(root, "keep.go"), "package keep\n// TODO keep\n")
	mustWrite(t, filepath.Join(root, "noise.skip"), "package noise\n// TODO skip-file\n")

	got, err := collectSearch(context.Background(), Options{
		Root:        root,
		Keyword:     "TODO",
		AllowedExts: []string{"go", "skip"},
		Workers:     1,
		NoGitignore: true,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("NoGitignore: got %d matches, want 2: %+v", len(got), got)
	}
}

func TestSearchEmptyKeyword(t *testing.T) {
	err := Search(context.Background(), Options{Root: ".", Keyword: ""}, make(chan Match))
	if err == nil {
		t.Errorf("expected error for empty keyword")
	}
}

func TestSearchNilResults(t *testing.T) {
	err := Search(context.Background(), Options{Root: ".", Keyword: "x"}, nil)
	if err == nil {
		t.Errorf("expected error for nil results")
	}
}

func TestSearchContextCancel(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n// TODO here\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := collectSearch(ctx, Options{
		Root:    root,
		Keyword: "TODO",
		Workers: 2,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search error = %v, want context.Canceled", err)
	}
}

func TestSearchDefaultRoot(t *testing.T) {
	// Empty Root should default to "."; use a temp cwd so we don't scan the repo.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n// TODO here\n")

	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	got, err := collectSearch(context.Background(), Options{
		Root:        "", // defaults to "."
		Keyword:     "TODO",
		AllowedExts: []string{"go"},
		Workers:     1,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1: %+v", len(got), got)
	}
}

func TestSearchWorkersDefault(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n// TODO here\n")

	// Workers <= 0 uses runtime.NumCPU(); ensure no panic/deadlock and a hit.
	got, err := collectSearch(context.Background(), Options{
		Root:    root,
		Keyword: "TODO",
		Workers: 0,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1: %+v", len(got), got)
	}
}

func TestSearchWithContextLines(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n// prev\n// TODO here\n// next\n")

	got, err := collectSearch(context.Background(), Options{
		Root:         root,
		Keyword:      "TODO",
		AllowedExts:  []string{"go"},
		Workers:      1,
		ContextLines: 1,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1: %+v", len(got), got)
	}
	m := got[0]
	if m.Line != 3 || m.Content != "// TODO here" {
		t.Errorf("match = %+v", m)
	}
	if !equalStrings(m.Before, []string{"// prev"}) {
		t.Errorf("Before = %v, want [// prev]", m.Before)
	}
	if !equalStrings(m.After, []string{"// next"}) {
		t.Errorf("After = %v, want [// next]", m.After)
	}
}

func TestSearchFileEmptyAndNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()

	emptyPath := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(emptyPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile empty: %v", err)
	}
	got, err := SearchFile(context.Background(), emptyPath, FileOptions{Keyword: "TODO"})
	if err != nil {
		t.Fatalf("empty SearchFile: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty: got %v, want none", got)
	}

	// Last line without trailing newline still matches.
	noNL := filepath.Join(dir, "nonewline.txt")
	if err := os.WriteFile(noNL, []byte("hello\n// TODO last"), 0o644); err != nil {
		t.Fatalf("WriteFile noNL: %v", err)
	}
	got, err = SearchFile(context.Background(), noNL, FileOptions{Keyword: "TODO"})
	if err != nil {
		t.Fatalf("noNL SearchFile: %v", err)
	}
	if len(got) != 1 || got[0].Line != 2 || got[0].Content != "// TODO last" {
		t.Fatalf("noNL: got %+v, want line 2 '// TODO last'", got)
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
		mu       sync.Mutex
		errPaths []string
	)
	got, err := collectSearch(context.Background(), Options{
		Root:    root,
		Keyword: "TODO",
		Workers: 2,
		OnError: func(path string, err error) {
			mu.Lock()
			errPaths = append(errPaths, path)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(errPaths) != 1 || errPaths[0] != bad {
		t.Errorf("OnError paths = %v, want [%q]", errPaths, bad)
	}
	if len(got) != 1 || got[0].Content != "// TODO ok" {
		t.Errorf("hits = %v, want one good hit", got)
	}
}

// collectSearch runs Search with a results channel and returns collected matches.
func collectSearch(ctx context.Context, opts Options) ([]Match, error) {
	results := make(chan Match, 32)
	var got []Match

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer close(results)
		return Search(ctx, opts, results)
	})
	g.Go(func() error {
		for m := range results {
			got = append(got, m)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return got, nil
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

func TestSearchFileRegex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "TODO first\nFIXME second\nnote only\nTODO again\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	matches, err := SearchFile(context.Background(), path, FileOptions{
		Keyword: `TODO|FIXME`,
		Regex:   true,
	})
	if err != nil {
		t.Fatalf("SearchFile: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("got %d matches, want 3: %+v", len(matches), matches)
	}
	if matches[0].Line != 1 || matches[1].Line != 2 || matches[2].Line != 4 {
		t.Errorf("lines = %d,%d,%d want 1,2,4", matches[0].Line, matches[1].Line, matches[2].Line)
	}
}

func TestSearchFileRegexIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("todo one\nTODO two\nToDo three\nnope\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Case-sensitive regex: only exact TODO
	got, err := SearchFile(context.Background(), path, FileOptions{
		Keyword: `TODO`,
		Regex:   true,
	})
	if err != nil {
		t.Fatalf("SearchFile: %v", err)
	}
	if len(got) != 1 || got[0].Line != 2 {
		t.Errorf("case-sensitive regex: got %+v, want 1 hit on line 2", got)
	}

	// Ignore-case regex
	got, err = SearchFile(context.Background(), path, FileOptions{
		Keyword:    `todo`,
		Regex:      true,
		IgnoreCase: true,
	})
	if err != nil {
		t.Fatalf("SearchFile: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("ignore-case regex: got %d hits, want 3: %+v", len(got), got)
	}
}

func TestSearchFileRegexContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	// lines: 1=alpha, 2=beta, 3=HIT one, 4=gamma, 5=HIT two, 6=delta
	content := "alpha\nbeta\nHIT one\ngamma\nHIT two\ndelta\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	matches, err := SearchFile(context.Background(), path, FileOptions{
		Keyword:      `HIT\s+\w+`,
		Regex:        true,
		ContextLines: 1,
	})
	if err != nil {
		t.Fatalf("SearchFile: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2: %+v", len(matches), matches)
	}
	if matches[0].Line != 3 || matches[0].Content != "HIT one" {
		t.Errorf("match0 = %+v", matches[0])
	}
	if !equalStrings(matches[0].Before, []string{"beta"}) {
		t.Errorf("match0.Before = %v, want [beta]", matches[0].Before)
	}
	if !equalStrings(matches[0].After, []string{"gamma"}) {
		t.Errorf("match0.After = %v, want [gamma]", matches[0].After)
	}
}

func TestSearchFileInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := SearchFile(context.Background(), path, FileOptions{
		Keyword: `[`,
		Regex:   true,
	})
	if err == nil {
		t.Fatal("expected invalid regex error")
	}
	if !strings.Contains(err.Error(), "invalid regex") {
		t.Errorf("error = %q, want it to mention invalid regex", err)
	}
}

func TestSearchInvalidRegex(t *testing.T) {
	// Fail before walk: bad pattern should not need a real tree of files.
	err := Search(context.Background(), Options{
		Root:    t.TempDir(),
		Keyword: `[`,
		Regex:   true,
	}, make(chan Match, 1))
	if err == nil {
		t.Fatal("expected invalid regex error")
	}
	if !strings.Contains(err.Error(), "invalid regex") {
		t.Errorf("error = %q, want it to mention invalid regex", err)
	}
}

func TestSearchRegex(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n// TODO here\n")
	mustWrite(t, filepath.Join(root, "b.go"), "package b\n// FIXME there\n")
	mustWrite(t, filepath.Join(root, "c.go"), "package c\n// note only\n")

	got, err := collectSearch(context.Background(), Options{
		Root:        root,
		Keyword:     `TODO|FIXME`,
		Regex:       true,
		AllowedExts: []string{"go"},
		Workers:     2,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d matches, want 2: %+v", len(got), got)
	}
}

func TestSearchOnFileDone(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n// TODO one\n// TODO two\n")
	mustWrite(t, filepath.Join(root, "b.go"), "package b\n// no hit\n")
	mustWrite(t, filepath.Join(root, "c.md"), "# TODO md\n") // filtered by ext

	var (
		mu    sync.Mutex
		files []string
		total int
	)
	got, err := collectSearch(context.Background(), Options{
		Root:        root,
		Keyword:     "TODO",
		AllowedExts: []string{"go"},
		Workers:     2,
		OnFileDone: func(path string, matchCount int) {
			mu.Lock()
			files = append(files, filepath.Base(path))
			total += matchCount
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("hits = %d, want 2", len(got))
	}
	if total != 2 {
		t.Errorf("OnFileDone total matchCount = %d, want 2", total)
	}
	// a.go and b.go only (c.md filtered by walker, never reaches workers).
	if len(files) != 2 {
		t.Fatalf("OnFileDone files = %v, want 2", files)
	}
	seen := map[string]bool{}
	for _, f := range files {
		seen[f] = true
	}
	if !seen["a.go"] || !seen["b.go"] {
		t.Errorf("OnFileDone files = %v, want a.go and b.go", files)
	}
}

func TestSearchOnFileDoneAfterIOError(t *testing.T) {
	// Per-file open failure should call OnFileDone(path, 0) so progress still advances.
	root := t.TempDir()
	good := filepath.Join(root, "good.go")
	bad := filepath.Join(root, "bad.go")
	mustWrite(t, good, "package good\n// TODO ok\n")
	mustWrite(t, bad, "package bad\n// TODO hidden by perms\n")

	if err := os.Chmod(bad, 0); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o644) })

	if f, err := os.Open(bad); err == nil {
		f.Close()
		t.Skip("unreadable file still openable (root?); skip OnFileDone I/O test")
	}

	var (
		mu       sync.Mutex
		done     = map[string]int{}
		errPaths []string
	)
	got, err := collectSearch(context.Background(), Options{
		Root:    root,
		Keyword: "TODO",
		Workers: 2,
		OnError: func(path string, err error) {
			mu.Lock()
			errPaths = append(errPaths, path)
			mu.Unlock()
		},
		OnFileDone: func(path string, matchCount int) {
			mu.Lock()
			done[path] = matchCount
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(errPaths) != 1 || errPaths[0] != bad {
		t.Errorf("OnError paths = %v, want [%q]", errPaths, bad)
	}
	if n, ok := done[bad]; !ok || n != 0 {
		t.Errorf("OnFileDone for bad file = (%v, %d), want (present, 0)", ok, n)
	}
	if n, ok := done[good]; !ok || n != 1 {
		t.Errorf("OnFileDone for good file = (%v, %d), want (present, 1)", ok, n)
	}
	if len(got) != 1 || got[0].Content != "// TODO ok" {
		t.Errorf("hits = %v, want one good hit", got)
	}
}

func TestSearchSameFileMatchesContiguous(t *testing.T) {
	// Matches from one file must be delivered contiguously (line order) even
	// with multiple workers, so consumers can coalesce context blocks.
	root := t.TempDir()
	const nFiles = 8
	const hitsPerFile = 5
	for i := 0; i < nFiles; i++ {
		var b strings.Builder
		b.WriteString("package p\n")
		for h := 0; h < hitsPerFile; h++ {
			b.WriteString("// TODO hit\n")
			b.WriteString("// filler\n")
		}
		mustWrite(t, filepath.Join(root, fmt.Sprintf("f%d.go", i)), b.String())
	}

	// Collect in arrival order (do not sort).
	results := make(chan Match, 64)
	var got []Match
	g, ctx := errgroup.WithContext(context.Background())
	g.Go(func() error {
		defer close(results)
		return Search(ctx, Options{
			Root:        root,
			Keyword:     "TODO",
			AllowedExts: []string{"go"},
			Workers:     4,
		}, results)
	})
	g.Go(func() error {
		for m := range results {
			got = append(got, m)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != nFiles*hitsPerFile {
		t.Fatalf("got %d hits, want %d", len(got), nFiles*hitsPerFile)
	}

	// Within each contiguous run of the same path, line numbers must be strictly
	// increasing and the run length must equal hitsPerFile (no interleaving).
	i := 0
	for i < len(got) {
		path := got[i].Path
		start := i
		lastLine := 0
		for i < len(got) && got[i].Path == path {
			if got[i].Line <= lastLine {
				t.Errorf("path %s: non-increasing lines at index %d: %d then %d",
					path, i, lastLine, got[i].Line)
			}
			lastLine = got[i].Line
			i++
		}
		run := i - start
		if run != hitsPerFile {
			t.Errorf("path %s: contiguous run length %d, want %d (interleaved?)", path, run, hitsPerFile)
		}
	}
}


