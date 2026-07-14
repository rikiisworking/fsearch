// Command fsearch is a fast recursive file content searcher.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/nick/fsearch/internal/output"
	"github.com/nick/fsearch/internal/searcher"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		exts         string
		ignores      []string
		ignoreCase   bool
		contextLines int
		workers      int
		noGitignore  bool
		noColor      bool
		regex        bool
		jsonOut      bool
	)

	cmd := &cobra.Command{
		Use:   "fsearch [keyword] [path]",
		Short: "Fast recursive file content search",
		Long: `fsearch searches for a keyword inside file contents under a path
(recursively, including child directories).

Matching is case-sensitive by default; use -i/--ignore-case to ignore case.
With -e/--regex, the keyword is a Go RE2 regular expression.
Output is grep-style (path:line:content). On a TTY, path/line/keyword are
colored; use --no-color or pipe to disable. -C N adds N lines of context
before and after each hit. --json emits one NDJSON object per match.

Examples:
  fsearch "TODO" .
  fsearch "TODO" . --ext go,md
  fsearch "FIXME" ./internal --ignore vendor
  fsearch "todo" . -i
  fsearch "TODO" . -C 2
  fsearch "TODO" . --ext go,md -C 1 -i
  fsearch "TODO" . --no-color
  fsearch "TODO" . --workers 4
  fsearch "TODO" . --no-gitignore
  fsearch 'TODO|FIXME' . -e
  fsearch 'todo' . -e -i
  fsearch "TODO" . --json`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if contextLines < 0 {
				return fmt.Errorf("context must be >= 0, got %d", contextLines)
			}
			if workers < 0 {
				return fmt.Errorf("workers must be >= 0, got %d", workers)
			}
			keyword := args[0]
			root := "."
			if len(args) > 1 {
				root = args[1]
			}
			opts := buildOptions(keyword, root, exts, ignores, ignoreCase, contextLines, regex)
			// 0 means searcher uses runtime.NumCPU() (existing library default).
			opts.Workers = workers
			opts.NoGitignore = noGitignore

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			return run(ctx, opts, cmd.OutOrStdout(), cmd.ErrOrStderr(), noColor, jsonOut)
		},
	}

	cmd.Flags().StringVar(&exts, "ext", "", "comma-separated file extensions to include (e.g. go,md)")
	cmd.Flags().StringArrayVar(&ignores, "ignore", nil, "basename or pattern to ignore (repeatable)")
	cmd.Flags().BoolVarP(&ignoreCase, "ignore-case", "i", false, "case-insensitive search")
	cmd.Flags().IntVarP(&contextLines, "context", "C", 0, "lines of context before and after each match")
	cmd.Flags().IntVar(&workers, "workers", 0, "number of concurrent file-search workers (0 = NumCPU)")
	cmd.Flags().BoolVar(&noGitignore, "no-gitignore", false, "do not load root .gitignore")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable colored output")
	cmd.Flags().BoolVarP(&regex, "regex", "e", false, "treat keyword as a Go RE2 regular expression")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit one NDJSON object per match on stdout")
	cmd.SilenceUsage = true

	return cmd
}

// buildOptions turns CLI args/flags into searcher.Options.
func buildOptions(keyword, root, exts string, ignores []string, ignoreCase bool, contextLines int, regex bool) searcher.Options {
	var skip []string
	for _, ig := range ignores {
		skip = append(skip, parseList(ig)...)
	}
	return searcher.Options{
		Root:         root,
		Keyword:      keyword,
		AllowedExts:  parseList(exts),
		SkipPatterns: skip,
		IgnoreCase:   ignoreCase,
		ContextLines: contextLines,
		Regex:        regex,
	}
}

// matchWriter formats matches for stdout (human Printer or JSONPrinter).
type matchWriter interface {
	WriteMatch(w io.Writer, m searcher.Match) error
	Flush(w io.Writer) error
}

// run executes search: hits go to stdout, skip warnings to stderr.
// If stderr is nil, warnings are discarded.
// noColor forces plain human output (also auto-disabled for non-TTY via fatih/color).
// jsonOut selects NDJSON (one object per match); colors are never applied in JSON mode.
func run(ctx context.Context, opts searcher.Options, stdout, stderr io.Writer, noColor, jsonOut bool) error {
	if stderr == nil {
		stderr = io.Discard
	}
	opts.OnError = func(path string, err error) {
		fmt.Fprintf(stderr, "fsearch: skip %s: %v\n", path, err)
	}

	// Compile/validate regex once (same (?i) rules as searcher) before workers start.
	// Highlight RE is only attached to the human printer; JSON has no highlight.
	var re *regexp.Regexp
	if opts.Regex {
		pattern := opts.Keyword
		if opts.IgnoreCase {
			pattern = "(?i)" + opts.Keyword
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid regex: %w", err)
		}
		if !jsonOut {
			re = compiled
		}
	}

	var printer matchWriter
	if jsonOut {
		printer = &output.JSONPrinter{}
	} else {
		printer = &output.Printer{
			Keyword:    opts.Keyword,
			IgnoreCase: opts.IgnoreCase,
			NoColor:    noColor,
			Regex:      re,
		}
	}

	results := make(chan searcher.Match, 64)
	g, ctx := errgroup.WithContext(ctx)

	// Producer: search sends matches, then we close the channel.
	g.Go(func() error {
		defer close(results)
		return searcher.Search(ctx, opts, results)
	})

	// Consumer: single writer to stdout (no mutex needed).
	g.Go(func() error {
		for m := range results {
			if err := printer.WriteMatch(stdout, m); err != nil {
				return err
			}
		}
		// Flush coalesced context groups buffered by the human printer.
		return printer.Flush(stdout)
	})

	return g.Wait()
}

// parseList splits a comma-separated flag value into trimmed non-empty parts.
// "" and "  " yield nil. "go, md" yields ["go", "md"].
func parseList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
