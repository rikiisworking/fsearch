package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/nick/fsearch/internal/searcher"
)

// jsonMatch is the NDJSON shape for one search hit.
// before/after are omitted when empty (no context or empty slices).
type jsonMatch struct {
	Path    string   `json:"path"`
	Line    int      `json:"line"`
	Content string   `json:"content"`
	Before  []string `json:"before,omitempty"`
	After   []string `json:"after,omitempty"`
}

// JSONPrinter writes one NDJSON object per Match (no colors, no coalescing).
// Each WriteMatch emits a single line: {"path":…,"line":…,"content":…}.
// Context lines appear as before/after arrays when present on the Match.
// Flush is a no-op; provided for symmetry with Printer.
type JSONPrinter struct{}

// WriteMatch marshals m as one JSON object followed by a newline.
func (p *JSONPrinter) WriteMatch(w io.Writer, m searcher.Match) error {
	obj := jsonMatch{
		Path:    m.Path,
		Line:    m.Line,
		Content: m.Content,
		Before:  m.Before,
		After:   m.After,
	}
	// Encode without HTML escape so path/content stay literal.
	// New encoder per call: Writer may differ between matches; Encode adds '\n'.
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(obj); err != nil {
		return fmt.Errorf("output: write json match: %w", err)
	}
	return nil
}

// Flush is a no-op for JSONPrinter (no buffered groups).
func (p *JSONPrinter) Flush(w io.Writer) error {
	return nil
}
