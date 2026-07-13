// Package searcher matches keywords inside file contents and reports line hits.
package searcher

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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

// fileOpError is a per-file I/O or scan failure. Search skips the file and
// may report it via OnError.
type fileOpError struct {
	op   string
	path string
	err  error
}

func (e *fileOpError) Error() string {
	return fmt.Sprintf("searcher: %s %s: %v", e.op, e.path, e.err)
}

func (e *fileOpError) Unwrap() error { return e.err }

func fileErr(op, path string, err error) error {
	return &fileOpError{op: op, path: path, err: err}
}

// Match is a single line hit inside a file.
type Match struct {
	Path    string   // file path as walked (usually absolute or as given)
	Line    int      // 1-based line number of the hit
	Content string   // full hit line text, without trailing newline
	Before  []string // context lines above the hit (empty when ContextLines == 0)
	After   []string // context lines below the hit (empty when ContextLines == 0)
}

// Options configures a multi-file concurrent search.
type Options struct {
	Root         string
	Keyword      string
	AllowedExts  []string // empty = all extensions
	SkipPatterns []string // basename ignore patterns
	Workers      int      // 0 = runtime.NumCPU()
	IgnoreCase   bool     // false = case-sensitive (default)
	ContextLines int      // lines of context before/after each hit; 0 = none

	// OnError is called when a file cannot be searched (open/read/scan).
	// Cancel errors are not reported here; Search returns ctx.Err() instead.
	// Optional: nil = silent skip. May be called concurrently from workers.
	OnError func(path string, err error)
}

// FileOptions configures a single-file search.
type FileOptions struct {
	Keyword      string
	IgnoreCase   bool
	ContextLines int // lines of context before/after each hit; 0 = none
}

// Search walks Root and searches files concurrently for Keyword.
// Matches are sent to results. The caller owns results (Search does not close it).
// The caller must consume results concurrently or risk deadlock when the buffer fills.
// Match order is not guaranteed.
func Search(ctx context.Context, opts Options, results chan<- Match) error {
	if strings.TrimSpace(opts.Keyword) == "" {
		return fmt.Errorf("searcher: keyword is required")
	}
	if opts.Root == "" {
		opts.Root = "."
	}
	if results == nil {
		return fmt.Errorf("searcher: results is nil")
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

	fileOpts := FileOptions{
		Keyword:      opts.Keyword,
		IgnoreCase:   opts.IgnoreCase,
		ContextLines: opts.ContextLines,
	}

	for i := 0; i < workers; i++ {
		g.Go(func() error {
			for path := range files {
				err := searchFile(ctx, path, fileOpts, results)
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					// Per-file I/O: skip + optional OnError.
					var fe *fileOpError
					if errors.As(err, &fe) {
						if opts.OnError != nil {
							opts.OnError(path, err)
						}
						continue
					}
					return err
				}
			}
			return nil
		})
	}

	return g.Wait()
}

// SearchFile scans path for literal keyword matches.
// Returns one Match per matching line. Line numbers are 1-based.
// Binary files (NUL in the first 8KiB) return nil, nil.
// When opts.ContextLines > 0, each Match includes Before/After context.
func SearchFile(ctx context.Context, path string, opts FileOptions) ([]Match, error) {
	results := make(chan Match, 32)
	var matches []Match
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for m := range results {
			matches = append(matches, m)
		}
	}()

	err := searchFile(ctx, path, opts, results)
	close(results)
	wg.Wait()
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// searchFile scans path and sends each hit to results (does not close results).
// Binary files (NUL in the first 8KiB) return nil without sending.
// When ContextLines == 0, streams line-by-line. When > 0, buffers lines so
// Before/After can be filled on each Match.
func searchFile(ctx context.Context, path string, opts FileOptions, results chan<- Match) error {
	if opts.Keyword == "" {
		return fmt.Errorf("searcher: keyword is required")
	}
	if results == nil {
		return fmt.Errorf("searcher: results is nil")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	f, err := os.Open(path)
	if err != nil {
		return fileErr("open", path, err)
	}
	defer f.Close()

	// Skip binary files: NUL in the first 8KiB.
	head := make([]byte, 8192)
	n, err := f.Read(head)
	if err != nil && err != io.EOF {
		return fileErr("read", path, err)
	}
	if bytes.IndexByte(head[:n], 0) >= 0 {
		return nil // binary: no matches, not an error
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fileErr("seek", path, err)
	}

	// Pre-lower keyword once when ignoring case.
	keyword := opts.Keyword
	if opts.IgnoreCase {
		keyword = strings.ToLower(keyword)
	}

	// Context path: buffer whole file so each hit can get Before/After.
	if opts.ContextLines > 0 {
		lines, err := readAllLines(ctx, f, path)
		if err != nil {
			return err
		}
		return emitMatches(ctx, path, lines, keyword, opts, results)
	}

	// Fast path: stream without buffering.
	scanner := newLineScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum%512 == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		line := scanner.Text()
		if !lineMatch(line, keyword, opts.IgnoreCase) {
			continue
		}
		m := Match{
			Path:    path,
			Line:    lineNum,
			Content: line,
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case results <- m:
		}
	}
	if err := scanner.Err(); err != nil {
		return fileErr("scan", path, err)
	}
	return nil
}

// emitMatches sends one Match per matching line in lines.
// Fills Before/After using opts.ContextLines (caller should only use when > 0).
func emitMatches(ctx context.Context, path string, lines []string, keyword string, opts FileOptions, results chan<- Match) error {
	n := opts.ContextLines
	for i, line := range lines {
		if i%512 == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if !lineMatch(line, keyword, opts.IgnoreCase) {
			continue
		}

		beforeStart := i - n
		if beforeStart < 0 {
			beforeStart = 0
		}
		var before []string
		if beforeStart < i {
			before = append([]string(nil), lines[beforeStart:i]...)
		}

		afterEnd := i + 1 + n
		if afterEnd > len(lines) {
			afterEnd = len(lines)
		}
		var after []string
		if i+1 < afterEnd {
			after = append([]string(nil), lines[i+1:afterEnd]...)
		}

		m := Match{
			Path:    path,
			Line:    i + 1,
			Content: line,
			Before:  before,
			After:   after,
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case results <- m:
		}
	}
	return nil
}

// readAllLines scans f into a slice of lines (without trailing newlines).
func readAllLines(ctx context.Context, f *os.File, path string) ([]string, error) {
	scanner := newLineScanner(f)
	var lines []string
	for scanner.Scan() {
		if len(lines)%512 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fileErr("scan", path, err)
	}
	return lines, nil
}

func newLineScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	// Allow longer lines than the default 64KiB token limit.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	return scanner
}

// lineMatch reports whether line contains keyword.
// When ignoreCase is true, keyword must already be lowercased by the caller.
func lineMatch(line, keyword string, ignoreCase bool) bool {
	if !ignoreCase {
		return strings.Contains(line, keyword)
	}
	return strings.Contains(strings.ToLower(line), keyword)
}
