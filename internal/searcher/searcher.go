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
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/nick/fsearch/internal/ignore"
	"github.com/nick/fsearch/internal/walker"
)

const (
	// binaryCheckSize is bytes read to detect binary files (NUL byte check).
	binaryCheckSize = 8192
	// cancelCheckInterval is how often to check context cancellation during scan.
	cancelCheckInterval = 512
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
	// Regex treats Keyword as a Go RE2 regular expression (literal match otherwise).
	Regex bool
	// NoGitignore skips loading root/.gitignore (defaults and --ignore still apply).
	NoGitignore bool

	// OnError is called when a path cannot be walked or a file cannot be
	// searched (open/read/scan). Cancel errors are not reported here; Search
	// returns ctx.Err() instead. Optional: nil = silent skip.
	// May be called concurrently from the walker and from workers.
	OnError func(path string, err error)

	// OnFileDone is called after each file is processed (hits, misses, binary
	// skip, or per-file I/O skip). matchCount is the number of line hits for
	// that file (0 on miss/binary/I/O skip). Cancel is not reported here.
	// Optional. May be called concurrently from workers.
	OnFileDone func(path string, matchCount int)
}

// FileOptions configures a single-file search.
type FileOptions struct {
	Keyword      string
	IgnoreCase   bool
	ContextLines int  // lines of context before/after each hit; 0 = none
	Regex        bool // treat Keyword as a Go RE2 regular expression
}

// Search walks Root and searches files concurrently for Keyword.
// Matches are sent to results. The caller owns results (Search does not close it).
// The caller must consume results concurrently or risk deadlock when the buffer fills.
// Matches from a single file are delivered contiguously (line order within the file).
// Global order across files is not sorted.
func Search(ctx context.Context, opts Options, results chan<- Match) error {
	if strings.TrimSpace(opts.Keyword) == "" {
		return fmt.Errorf("searcher: keyword is required")
	}
	// Fail fast on bad regex before walk/workers start.
	if _, err := newMatcher(opts.Keyword, opts.IgnoreCase, opts.Regex); err != nil {
		return err
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
	// Load root .gitignore when present (missing file is not an error).
	if !opts.NoGitignore {
		giPath := filepath.Join(opts.Root, ".gitignore")
		if gi, err := ignore.LoadGitignoreFile(giPath); err != nil {
			// Non-missing I/O failure: surface via OnError if set, else ignore quietly.
			if opts.OnError != nil {
				opts.OnError(giPath, err)
			}
		} else if gi != nil {
			filter.SetGitignore(opts.Root, gi)
		}
	}
	files := make(chan string, workers*2)

	g, ctx := errgroup.WithContext(ctx)

	// Producer: walk tree, send paths, then close channel.
	g.Go(func() error {
		defer close(files)
		// Reuse OnError for walk entry failures (e.g. permission denied on a dir).
		return walker.Walk(ctx, opts.Root, filter, files, opts.OnError)
	})

	fileOpts := FileOptions{
		Keyword:      opts.Keyword,
		IgnoreCase:   opts.IgnoreCase,
		ContextLines: opts.ContextLines,
		Regex:        opts.Regex,
	}

	// emitMu ensures all matches for one file are sent contiguously on results
	// so consumers can coalesce overlapping context blocks.
	var emitMu sync.Mutex

	for i := 0; i < workers; i++ {
		g.Go(func() error {
			for path := range files {
				n, err := searchFile(ctx, path, fileOpts, results, &emitMu)
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					// Per-file I/O: skip + optional OnError / OnFileDone.
					var fe *fileOpError
					if errors.As(err, &fe) {
						if opts.OnError != nil {
							opts.OnError(path, err)
						}
						if opts.OnFileDone != nil {
							opts.OnFileDone(path, 0)
						}
						continue
					}
					return err
				}
				if opts.OnFileDone != nil {
					opts.OnFileDone(path, n)
				}
			}
			return nil
		})
	}

	return g.Wait()
}

// SearchFile scans path for keyword (or regex) matches.
// Returns one Match per matching line. Line numbers are 1-based.
// Binary files (NUL in the first 8KiB) return nil, nil.
// When opts.ContextLines > 0, each Match includes Before/After context.
// When opts.Regex is true, Keyword is compiled as a Go RE2 pattern;
// invalid patterns return an error.
func SearchFile(ctx context.Context, path string, opts FileOptions) ([]Match, error) {
	return scanFile(ctx, path, opts)
}

// searchFile scans path and sends each hit to results (does not close results).
// Returns the number of matches sent (0 for binary/no hits).
// Binary files (NUL in the first 8KiB) return 0, nil without sending.
// When ContextLines == 0, scans line-by-line then batch-sends matches.
// When > 0, buffers lines so Before/After can be filled on each Match.
// If emitMu is non-nil, all matches for this file are sent under that lock so
// they appear contiguously on results.
func searchFile(ctx context.Context, path string, opts FileOptions, results chan<- Match, emitMu *sync.Mutex) (int, error) {
	if results == nil {
		return 0, fmt.Errorf("searcher: results is nil")
	}
	matches, err := scanFile(ctx, path, opts)
	if err != nil {
		return 0, err
	}
	if err := sendMatches(ctx, results, matches, emitMu); err != nil {
		return 0, err
	}
	return len(matches), nil
}

// scanFile opens path and returns all keyword/regex hits (no channel).
// Binary files return (nil, nil). Keyword must be non-empty.
// Invalid regex patterns return an error before scanning file contents.
func scanFile(ctx context.Context, path string, opts FileOptions) ([]Match, error) {
	m, err := newMatcher(opts.Keyword, opts.IgnoreCase, opts.Regex)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fileErr("open", path, err)
	}
	defer f.Close()

	// Skip binary files: NUL in the first binaryCheckSize bytes.
	head := make([]byte, binaryCheckSize)
	n, err := f.Read(head)
	if err != nil && err != io.EOF {
		return nil, fileErr("read", path, err)
	}
	if bytes.IndexByte(head[:n], 0) >= 0 {
		return nil, nil // binary: no matches, not an error
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fileErr("seek", path, err)
	}

	// Context path: buffer whole file so each hit can get Before/After.
	if opts.ContextLines > 0 {
		lines, err := readAllLines(ctx, f, path)
		if err != nil {
			return nil, err
		}
		return collectContextMatches(ctx, path, lines, m, opts.ContextLines)
	}

	// Fast path: scan without full-file context buffers.
	scanner := newLineScanner(f)
	lineNum := 0
	var matches []Match
	for scanner.Scan() {
		lineNum++
		if lineNum%cancelCheckInterval == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		line := scanner.Text()
		if !m.match(line) {
			continue
		}
		matches = append(matches, Match{
			Path:    path,
			Line:    lineNum,
			Content: line,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fileErr("scan", path, err)
	}
	return matches, nil
}

// collectContextMatches builds one Match per matching line with Before/After filled.
func collectContextMatches(ctx context.Context, path string, lines []string, m matcher, contextLines int) ([]Match, error) {
	n := contextLines
	var matches []Match
	for i, line := range lines {
		if i%cancelCheckInterval == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		if !m.match(line) {
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

		matches = append(matches, Match{
			Path:    path,
			Line:    i + 1,
			Content: line,
			Before:  before,
			After:   after,
		})
	}
	return matches, nil
}

// sendMatches writes matches to results. If emitMu is non-nil, holds it for the whole send.
func sendMatches(ctx context.Context, results chan<- Match, matches []Match, emitMu *sync.Mutex) error {
	if len(matches) == 0 {
		return nil
	}
	if emitMu != nil {
		emitMu.Lock()
		defer emitMu.Unlock()
	}
	for _, m := range matches {
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
		if len(lines)%cancelCheckInterval == 0 {
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
