// Command fsearch is a fast recursive file content searcher.
package main

import (
	"context"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

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
		exts    string
		ignores []string
	)

	cmd := &cobra.Command{
		Use:   "fsearch [keyword] [path]",
		Short: "Fast recursive file content search",
		Long: `fsearch searches for a keyword inside file contents under a path
(recursively, including child directories).

Examples:
  fsearch "TODO" .
  fsearch "TODO" . --ext go,md
  fsearch "FIXME" ./internal --ignore vendor`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := args[0]
			root := "."
			if len(args) > 1 {
				root = args[1]
			}

			var allowedExts []string
			if strings.TrimSpace(exts) != "" {
				for _, e := range strings.Split(exts, ",") {
					e = strings.TrimSpace(e)
					if e != "" {
						allowedExts = append(allowedExts, e)
					}
				}
			}

			var skipPatterns []string
			for _, ig := range ignores {
				for _, p := range strings.Split(ig, ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						skipPatterns = append(skipPatterns, p)
					}
				}
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			return searcher.Search(ctx, searcher.Options{
				Root:         root,
				Keyword:      keyword,
				AllowedExts:  allowedExts,
				SkipPatterns: skipPatterns,
			}, func(m searcher.Match) error {
				return output.WriteMatch(cmd.OutOrStdout(), m.Path, m.Line, m.Content)
			})
		},
	}

	cmd.Flags().StringVar(&exts, "ext", "", "comma-separated file extensions to include (e.g. go,md)")
	cmd.Flags().StringArrayVar(&ignores, "ignore", nil, "basename or pattern to ignore (repeatable)")
	cmd.SilenceUsage = true

	return cmd
}
