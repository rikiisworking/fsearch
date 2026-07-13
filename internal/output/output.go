// Package output formats and prints search results for the terminal.
package output

import (
	"fmt"
	"io"

	"github.com/nick/fsearch/internal/searcher"
)

// Printer formats Match values for the terminal.
// Keyword/IgnoreCase/NoColor are reserved for later color highlighting steps.
type Printer struct {
	Keyword    string
	IgnoreCase bool
	NoColor    bool

	// written is the number of matches printed so far (for context separators).
	written int
}

// WriteMatch prints one hit in grep-style format.
//
// Without context:
//
//	path:line:content
//
// With Before/After context:
//
//	path-line:before
//	path:line:content
//	path-line:after
//
// A "--" separator is printed before each match that has context after the first
// match written by this Printer.
func (p *Printer) WriteMatch(w io.Writer, m searcher.Match) error {
	if p == nil {
		p = &Printer{}
	}

	hasContext := len(m.Before) > 0 || len(m.After) > 0
	if hasContext && p.written > 0 {
		if _, err := fmt.Fprint(w, "--\n"); err != nil {
			return fmt.Errorf("output: write separator: %w", err)
		}
	}

	// Before context: path-line:text
	for i, line := range m.Before {
		lineNo := m.Line - len(m.Before) + i
		if err := writeContextLine(w, m.Path, lineNo, line); err != nil {
			return err
		}
	}

	// Hit line: path:line:text
	if _, err := fmt.Fprintf(w, "%s:%d:%s\n", m.Path, m.Line, m.Content); err != nil {
		return fmt.Errorf("output: write match: %w", err)
	}

	// After context: path-line:text
	for i, line := range m.After {
		if err := writeContextLine(w, m.Path, m.Line+1+i, line); err != nil {
			return err
		}
	}

	p.written++
	return nil
}

func writeContextLine(w io.Writer, path string, lineNo int, content string) error {
	if _, err := fmt.Fprintf(w, "%s-%d-%s\n", path, lineNo, content); err != nil {
		return fmt.Errorf("output: write context: %w", err)
	}
	return nil
}

// WriteMatch prints one hit using a default Printer (no keyword/color options).
// Prefer constructing a Printer when formatting many matches.
func WriteMatch(w io.Writer, m searcher.Match) error {
	return (&Printer{}).WriteMatch(w, m)
}
