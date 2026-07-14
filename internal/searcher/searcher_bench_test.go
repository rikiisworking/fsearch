package searcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// benchWrite writes content to path for benchmarks (creates parent dirs).
func benchWrite(b *testing.B, path, content string) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatalf("WriteFile: %v", err)
	}
}

// benchTree builds a temp tree of nFiles .go files, each with nLines lines.
// Every hitEvery-th line (0-based) contains the keyword "TODO".
// hitEvery <= 0 means no hits.
func benchTree(b *testing.B, nFiles, nLines, hitEvery int) string {
	b.Helper()
	root := b.TempDir()
	for i := 0; i < nFiles; i++ {
		var sb strings.Builder
		sb.Grow(nLines * 16)
		for j := 0; j < nLines; j++ {
			if hitEvery > 0 && j%hitEvery == 0 {
				sb.WriteString("// TODO hit\n")
			} else {
				fmt.Fprintf(&sb, "// line %d\n", j)
			}
		}
		path := filepath.Join(root, fmt.Sprintf("f%04d.go", i))
		benchWrite(b, path, sb.String())
	}
	return root
}

// BenchmarkSearch walks a fixed tree and counts matches (no context lines).
// Tree is built once; each iteration runs a full Search.
func BenchmarkSearch(b *testing.B) {
	// Modest size: snappy locally, still exercises walk + workers + scan.
	const (
		nFiles   = 50
		nLines   = 200
		hitEvery = 20
	)
	root := benchTree(b, nFiles, nLines, hitEvery)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := benchRunSearch(b, Options{
			Root:        root,
			Keyword:     "TODO",
			AllowedExts: []string{"go"},
			Workers:     0, // NumCPU
		})
		if err != nil {
			b.Fatalf("Search: %v", err)
		}
		if n == 0 {
			b.Fatal("expected hits, got 0")
		}
	}
}

// BenchmarkSearchWithContext is the same tree with -C 1 (full-file buffer path).
func BenchmarkSearchWithContext(b *testing.B) {
	const (
		nFiles   = 50
		nLines   = 200
		hitEvery = 20
	)
	root := benchTree(b, nFiles, nLines, hitEvery)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := benchRunSearch(b, Options{
			Root:         root,
			Keyword:      "TODO",
			AllowedExts:  []string{"go"},
			Workers:      0,
			ContextLines: 1,
		})
		if err != nil {
			b.Fatalf("Search: %v", err)
		}
		if n == 0 {
			b.Fatal("expected hits, got 0")
		}
	}
}

// benchRunSearch runs Search and returns the number of matches.
func benchRunSearch(b *testing.B, opts Options) (int, error) {
	b.Helper()
	results := make(chan Match, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Search(context.Background(), opts, results)
		close(results)
	}()

	n := 0
	for range results {
		n++
	}
	return n, <-errCh
}
