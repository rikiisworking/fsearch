// Package searcher matches keywords inside file contents and reports line hits.
package searcher

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/nick/fsearch/internal/ignore"
	"github.com/nick/fsearch/internal/walker"
)

// Match is a single line hit inside a file.
type Match struct {
	Path    string // file path as walked (usually absolute or as given)
	Line    int    // 1-based line number
	Content string // full line text, without trailing newline
}

// Options configures a multi-file concurrent search.
type Options struct {
	Root         string
	Keyword      string
	AllowedExts  []string // empty = all extensions
	SkipPatterns []string // basename ignore patterns
	Workers      int      // 0 = runtime.NumCPU()
}

// Search walks Root and searches files concurrently for Keyword.
// emit is called for each match (serialized with a mutex; safe for stdout).
// Match order is not guaranteed.
func Search(ctx context.Context, opts Options, emit func(Match) error) error {
	if strings.TrimSpace(opts.Keyword) == "" {
		return fmt.Errorf("searcher: keyword is required")
	}
	if opts.Root == "" {
		opts.Root = "."
	}
	if emit == nil {
		return fmt.Errorf("searcher: emit is nil")
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	filter := ignore.New(opts.AllowedExts, opts.SkipPatterns)
	files := make(chan string, workers*2)

	g, ctx := errgroup.WithContext(ctx)

	// Producer: walk tree, send paths, then close channel.
	g.Go(func() error {
		defer close(files)
		return walker.Walk(ctx, opts.Root, filter, files)
	})

	var emitMu sync.Mutex
	for i := 0; i < workers; i++ {
		g.Go(func() error {
			for path := range files {
				matches, err := SearchFile(ctx, path, opts.Keyword)
				if err != nil {
					// Unreadable / scan errors: skip file unless cancelled.
					if ctx.Err() != nil {
						return ctx.Err()
					}
					continue
				}
				for _, m := range matches {
					emitMu.Lock()
					err := emit(m)
					emitMu.Unlock()
					if err != nil {
						return err
					}
				}
			}
			return nil
		})
	}

	return g.Wait()
}

// SearchFile scans path for literal keyword matches (case-sensitive).
// Returns one Match per matching line. Line numbers are 1-based.
// Binary files (NUL in the first 8KiB) return nil, nil.
func SearchFile(ctx context.Context, path, keyword string) ([]Match, error) {
	if keyword == "" {
		return nil, fmt.Errorf("searcher: keyword is required")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("searcher: open %s: %w", path, err)
	}
	defer f.Close()

	// Skip binary files: NUL in the first 8KiB.
	head := make([]byte, 8192)
	n, err := f.Read(head)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("searcher: read %s: %w", path, err)
	}
	if bytes.IndexByte(head[:n], 0) >= 0 {
		return nil, nil // binary: no matches, not an error
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("searcher: seek %s: %w", path, err)
	}

	scanner := bufio.NewScanner(f)
	// Allow longer lines than the default 64KiB token limit.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var matches []Match
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		// Cheap cancel check every so often (not every line).
		if lineNum%512 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		line := scanner.Text()
		if strings.Contains(line, keyword) {
			matches = append(matches, Match{
				Path:    path,
				Line:    lineNum,
				Content: line,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("searcher: scan %s: %w", path, err)
	}
	return matches, nil
}
