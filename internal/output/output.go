// Package output formats and prints search results for the terminal.
package output

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/fatih/color"

	"github.com/nick/fsearch/internal/searcher"
)

// Printer formats Match values for the terminal.
// Keyword and IgnoreCase drive hit-line keyword highlighting.
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
// When color is enabled: path is magenta, line number is green, and keyword
// occurrences on the hit line are bold red. Context lines are not keyword-highlighted.
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

	// Before context: path-line:text (no keyword highlight)
	for i, line := range m.Before {
		lineNo := m.Line - len(m.Before) + i
		if err := p.writeLine(w, m.Path, lineNo, line, '-', true); err != nil {
			return err
		}
	}

	// Hit line: path:line:text (keyword highlighted when color on)
	if err := p.writeLine(w, m.Path, m.Line, m.Content, ':', false); err != nil {
		return err
	}

	// After context: path-line:text (no keyword highlight)
	for i, line := range m.After {
		if err := p.writeLine(w, m.Path, m.Line+1+i, line, '-', true); err != nil {
			return err
		}
	}

	p.written++
	return nil
}

// writeLine writes path{sep}line{sep}content\n with optional color on path/line.
// Keyword highlight is applied only for hit lines (isContext == false).
func (p *Printer) writeLine(w io.Writer, path string, lineNo int, content string, sep byte, isContext bool) error {
	pathPart := p.colorPath(path, isContext)
	linePart := p.colorLine(lineNo, isContext)
	contentPart := content
	if !isContext {
		contentPart = p.highlightContent(content)
	}
	if _, err := fmt.Fprintf(w, "%s%c%s%c%s\n", pathPart, sep, linePart, sep, contentPart); err != nil {
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

// highlightContent wraps keyword occurrences in bold red when color is enabled.
// When color is off or keyword is empty, returns content unchanged.
func (p *Printer) highlightContent(content string) string {
	return highlight(content, p.Keyword, p.IgnoreCase, p.useColor())
}

// highlight returns content with each non-overlapping keyword occurrence wrapped
// in bold red ANSI codes when enabled is true. Empty keyword returns content as-is.
// Matching uses strings.Contains semantics: case-sensitive unless ignoreCase.
func highlight(content, keyword string, ignoreCase, enabled bool) string {
	if !enabled || keyword == "" {
		return content
	}

	var b strings.Builder
	rest := content
	kwLen := len(keyword)
	// Lowercase forms for case-insensitive index search; emit original slices.
	kwFind := keyword
	if ignoreCase {
		kwFind = strings.ToLower(keyword)
	}

	for rest != "" {
		hay := rest
		if ignoreCase {
			hay = strings.ToLower(rest)
		}
		i := strings.Index(hay, kwFind)
		if i < 0 {
			b.WriteString(rest)
			break
		}
		// Prefix before match (original casing).
		b.WriteString(rest[:i])
		// Matched span from original string (preserve casing).
		match := rest[i : i+kwLen]
		// EnableColor on this instance so highlight works even when the
		// global color.NoColor is true (caller already gated with enabled).
		c := color.New(color.FgRed, color.Bold)
		c.EnableColor()
		b.WriteString(c.Sprint(match))
		rest = rest[i+kwLen:]
	}
	return b.String()
}

// WriteMatch prints one hit using a plain (no-color) Printer.
// Prefer constructing a Printer when formatting many matches.
func WriteMatch(w io.Writer, m searcher.Match) error {
	return (&Printer{NoColor: true}).WriteMatch(w, m)
}
