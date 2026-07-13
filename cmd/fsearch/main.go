// Command fsearch is a fast recursive file content searcher.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
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
		exts       string
		ignores    []string
		ignoreCase bool
	)

	cmd := &cobra.Command{
		Use:   "fsearch [keyword] [path]",
		Short: "Fast recursive file content search",
		Long: `fsearch searches for a keyword inside file contents under a path
(recursively, including child directories).

Examples:
  fsearch "TODO" .
  fsearch "TODO" . --ext go,md
  fsearch "FIXME" ./internal --ignore vendor
  fsearch "todo" . -i`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := args[0]
			root := "."
			if len(args) > 1 {
				root = args[1]
			}
			opts := buildOptions(keyword, root, exts, ignores, ignoreCase)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			return run(ctx, opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	cmd.Flags().StringVar(&exts, "ext", "", "comma-separated file extensions to include (e.g. go,md)")
	cmd.Flags().StringArrayVar(&ignores, "ignore", nil, "basename or pattern to ignore (repeatable)")
	cmd.Flags().BoolVarP(&ignoreCase, "ignore-case", "i", false, "case-insensitive search")
	cmd.SilenceUsage = true

	return cmd
}

// buildOptions turns CLI args/flags into searcher.Options.
func buildOptions(keyword, root, exts string, ignores []string, ignoreCase bool) searcher.Options {
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
	}
}

// run executes search: hits go to stdout, skip warnings to stderr.
// If stderr is nil, warnings are discarded.
func run(ctx context.Context, opts searcher.Options, stdout, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	opts.OnError = func(path string, err error) {
		fmt.Fprintf(stderr, "fsearch: skip %s: %v\n", path, err)
	}

	results := make(chan searcher.Match, 64)
	g, ctx := errgroup.WithContext(ctx)

	// Producer: search sends matches, then we close the channel.
	g.Go(func() error {
		defer close(results)
		return searcher.Search(ctx, opts, results)
	})

	// Consumer: single writer to stdout (no mutex needed).
	printer := &output.Printer{
		Keyword:    opts.Keyword,
		IgnoreCase: opts.IgnoreCase,
	}
	g.Go(func() error {
		for m := range results {
			if err := printer.WriteMatch(stdout, m); err != nil {
				return err
			}
		}
		return nil
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
