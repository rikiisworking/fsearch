// Package output formats and prints search results for the terminal.
package output

import (
	"fmt"
	"io"

	"github.com/nick/fsearch/internal/searcher"
)

// WriteMatch prints one hit in grep-style format: path:line:content
// Before/After context lines are ignored until Sprint 2 formatting.
func WriteMatch(w io.Writer, m searcher.Match) error {
	_, err := fmt.Fprintf(w, "%s:%d:%s\n", m.Path, m.Line, m.Content)
	if err != nil {
		return fmt.Errorf("output: write match: %w", err)
	}
	return nil
}
