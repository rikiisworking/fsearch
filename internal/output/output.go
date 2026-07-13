// Package output formats and prints search results for the terminal.
package output

import (
	"fmt"
	"io"
	"strconv"

	"github.com/fatih/color"

	"github.com/nick/fsearch/internal/searcher"
)

// Printer formats Match values for the terminal.
// Keyword and IgnoreCase are used for hit-line keyword highlighting (later).
// NoColor forces plain text; otherwise colors follow TTY / NO_COLOR via fatih/color.
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
//
// When color is enabled, path is magenta and line number is green. Content is
// left uncolored (keyword highlight lands in a later step).
func (p *Printer) WriteMatch(w io.Writer, m searcher.Match) error {
	if p == nil {
		p = &Printer{NoColor: true}
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
		if err := p.writeLine(w, m.Path, lineNo, line, '-', true); err != nil {
			return err
		}
	}

	// Hit line: path:line:text
	if err := p.writeLine(w, m.Path, m.Line, m.Content, ':', false); err != nil {
		return err
	}

	// After context: path-line:text
	for i, line := range m.After {
		if err := p.writeLine(w, m.Path, m.Line+1+i, line, '-', true); err != nil {
			return err
		}
	}

	p.written++
	return nil
}

// writeLine writes path{sep}line{sep}content\n with optional color on path/line.
func (p *Printer) writeLine(w io.Writer, path string, lineNo int, content string, sep byte, isContext bool) error {
	pathPart := p.colorPath(path, isContext)
	linePart := p.colorLine(lineNo, isContext)
	if _, err := fmt.Fprintf(w, "%s%c%s%c%s\n", pathPart, sep, linePart, sep, content); err != nil {
		return fmt.Errorf("output: write line: %w", err)
	}
	return nil
}

func (p *Printer) useColor() bool {
	if p.NoColor {
		return false
	}
	// Respect fatih/color's TTY + NO_COLOR detection.
	return !color.NoColor
}

func (p *Printer) colorPath(path string, isContext bool) string {
	if !p.useColor() {
		return path
	}
	c := color.New(color.FgMagenta)
	if isContext {
		c.Add(color.Faint)
	}
	return c.Sprint(path)
}

func (p *Printer) colorLine(lineNo int, isContext bool) string {
	s := strconv.Itoa(lineNo)
	if !p.useColor() {
		return s
	}
	c := color.New(color.FgGreen)
	if isContext {
		c.Add(color.Faint)
	}
	return c.Sprint(s)
}

// WriteMatch prints one hit using a plain (no-color) Printer.
// Prefer constructing a Printer when formatting many matches.
func WriteMatch(w io.Writer, m searcher.Match) error {
	return (&Printer{NoColor: true}).WriteMatch(w, m)
}
