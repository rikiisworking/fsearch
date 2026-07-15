// Package output formats and prints search results for the terminal.
package output

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/fatih/color"

	"github.com/nick/fsearch/internal/searcher"
)

// Shared color instances (hot path: many lines). EnableColor so Sprint paints
// even when the process-wide color.NoColor is true; callers still gate with useColor.
var (
	colorPathNormal = sync.OnceValue(func() *color.Color {
		c := color.New(color.FgMagenta)
		c.EnableColor()
		return c
	})
	colorPathFaint = sync.OnceValue(func() *color.Color {
		c := color.New(color.FgMagenta, color.Faint)
		c.EnableColor()
		return c
	})
	colorLineNormal = sync.OnceValue(func() *color.Color {
		c := color.New(color.FgGreen)
		c.EnableColor()
		return c
	})
	colorLineFaint = sync.OnceValue(func() *color.Color {
		c := color.New(color.FgGreen, color.Faint)
		c.EnableColor()
		return c
	})
	colorHit = sync.OnceValue(func() *color.Color {
		c := color.New(color.FgRed, color.Bold)
		c.EnableColor()
		return c
	})
)

// Printer formats Match values for the terminal.
// Keyword and IgnoreCase drive hit-line keyword highlighting in literal mode.
// When Regex is non-nil, hit-line spans come from that pattern instead
// (IgnoreCase should already be baked into the compiled expression, e.g. (?i)).
// NoColor forces plain text; otherwise colors follow TTY / NO_COLOR via fatih/color.
//
// When matches include context, overlapping or adjacent blocks on the same path
// are buffered and flushed as one grep-style group: each line printed once,
// with ":" for hit lines and "-" for context. Call Flush after the last match.
// Requires same-file matches to arrive contiguously.
type Printer struct {
	Keyword    string
	IgnoreCase bool
	NoColor    bool
	// Regex, when non-nil, highlights matches of this pattern on hit lines
	// instead of literal Keyword/IgnoreCase.
	Regex *regexp.Regexp

	// groupsWritten counts flushed context groups / plain hits (for "--").
	groupsWritten int
	// pending holds an open coalescing group of context matches (same path).
	pending []searcher.Match
}

// WriteMatch prints one hit in grep-style format.
//
// Without context:
//
//	path:line:content
//
// With Before/After context (after Flush of the coalesced group):
//
//	path-line-before
//	path:line:content
//	path-line-after
//
// A "--" separator is printed between non-overlapping context groups. Overlapping
// or adjacent groups on the same path are merged without a separator.
//
// Context matches may be deferred until a non-coalescing match arrives or Flush
// is called so hit lines inside another match's context print with ":" not "-".
//
// When color is enabled: path is magenta, line number is green, and keyword
// occurrences on the hit line are bold red. Context lines are not keyword-highlighted.
func (p *Printer) WriteMatch(w io.Writer, m searcher.Match) error {
	if p == nil {
		p = &Printer{NoColor: true}
	}

	hasContext := len(m.Before) > 0 || len(m.After) > 0
	if !hasContext {
		if err := p.flushPending(w); err != nil {
			return err
		}
		if err := p.writeLine(w, m.Path, m.Line, m.Content, ':', false); err != nil {
			return err
		}
		p.groupsWritten++
		return nil
	}

	if len(p.pending) > 0 {
		if m.Path != p.pending[0].Path || blockStart(m) > pendingEnd(p.pending)+1 {
			if err := p.flushPending(w); err != nil {
				return err
			}
		}
	}
	p.pending = append(p.pending, m)
	return nil
}

// Flush writes any buffered context group. Call after the last WriteMatch.
func (p *Printer) Flush(w io.Writer) error {
	if p == nil {
		return nil
	}
	return p.flushPending(w)
}

func blockStart(m searcher.Match) int {
	return m.Line - len(m.Before)
}

func blockEnd(m searcher.Match) int {
	return m.Line + len(m.After)
}

func pendingEnd(pending []searcher.Match) int {
	end := blockEnd(pending[0])
	for _, m := range pending[1:] {
		if e := blockEnd(m); e > end {
			end = e
		}
	}
	return end
}

// flushPending renders one coalesced context group (grep-style).
func (p *Printer) flushPending(w io.Writer) error {
	if len(p.pending) == 0 {
		return nil
	}

	if p.groupsWritten > 0 {
		if _, err := fmt.Fprint(w, "--\n"); err != nil {
			return fmt.Errorf("output: write separator: %w", err)
		}
	}

	path := p.pending[0].Path
	start := blockStart(p.pending[0])
	end := pendingEnd(p.pending)

	// line text and which line numbers are hits
	text := make(map[int]string, end-start+1)
	hits := make(map[int]struct{}, len(p.pending))
	for _, m := range p.pending {
		hits[m.Line] = struct{}{}
		s := blockStart(m)
		for i, line := range m.Before {
			text[s+i] = line
		}
		text[m.Line] = m.Content
		for i, line := range m.After {
			text[m.Line+1+i] = line
		}
	}

	for lineNo := start; lineNo <= end; lineNo++ {
		line, ok := text[lineNo]
		if !ok {
			continue
		}
		if _, isHit := hits[lineNo]; isHit {
			if err := p.writeLine(w, path, lineNo, line, ':', false); err != nil {
				return err
			}
		} else {
			if err := p.writeLine(w, path, lineNo, line, '-', true); err != nil {
				return err
			}
		}
	}

	p.pending = p.pending[:0]
	p.groupsWritten++
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
	if isContext {
		return colorPathFaint().Sprint(path)
	}
	return colorPathNormal().Sprint(path)
}

func (p *Printer) colorLine(lineNo int, isContext bool) string {
	s := strconv.Itoa(lineNo)
	if !p.useColor() {
		return s
	}
	if isContext {
		return colorLineFaint().Sprint(s)
	}
	return colorLineNormal().Sprint(s)
}

// highlightContent wraps keyword/regex occurrences in bold red when color is enabled.
// When color is off, returns content unchanged.
func (p *Printer) highlightContent(content string) string {
	if p == nil || !p.useColor() {
		return content
	}
	if p.Regex != nil {
		return highlightSpans(content, p.Regex.FindAllStringIndex(content, -1))
	}
	return highlight(content, p.Keyword, p.IgnoreCase, true)
}

// highlight returns content with each non-overlapping keyword occurrence wrapped
// in bold red ANSI codes when enabled is true. Empty keyword returns content as-is.
// Matching uses strings.Contains semantics: case-sensitive unless ignoreCase.
// Ignore-case spans are found in original coordinates (never slice with ToLower indices).
func highlight(content, keyword string, ignoreCase, enabled bool) string {
	if !enabled || keyword == "" {
		return content
	}

	var spans [][]int
	rest := content
	offset := 0
	for rest != "" {
		start, end, ok := indexKeyword(rest, keyword, ignoreCase)
		if !ok {
			break
		}
		spans = append(spans, []int{offset + start, offset + end})
		rest = rest[end:]
		offset += end
	}
	return highlightSpans(content, spans)
}

// highlightSpans wraps each [start,end) span of content in bold red ANSI codes.
// Spans must be non-overlapping and sorted by start (as from FindAllStringIndex
// or sequential literal search). Empty or invalid spans are skipped.
func highlightSpans(content string, spans [][]int) string {
	if len(spans) == 0 {
		return content
	}

	hit := colorHit()
	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		if len(sp) < 2 {
			continue
		}
		start, end := sp[0], sp[1]
		if start < pos || end > len(content) || start >= end {
			continue
		}
		b.WriteString(content[pos:start])
		b.WriteString(hit.Sprint(content[start:end]))
		pos = end
	}
	b.WriteString(content[pos:])
	return b.String()
}

// indexKeyword finds the first keyword occurrence in s.
// Returns [start, end) byte offsets in s, or ok=false if none.
// When ignoreCase, matching follows strings.ToLower (same idea as searcher.lineMatch)
// but maps the hit back onto original byte spans so length-changing folds are safe.
func indexKeyword(s, keyword string, ignoreCase bool) (start, end int, ok bool) {
	if keyword == "" {
		return 0, 0, false
	}
	if !ignoreCase {
		i := strings.Index(s, keyword)
		if i < 0 {
			return 0, 0, false
		}
		return i, i + len(keyword), true
	}

	kw := strings.ToLower(keyword)
	// When ToLower does not change length, lowered indices match original bytes.
	if lower := strings.ToLower(s); len(lower) == len(s) {
		i := strings.Index(lower, kw)
		if i < 0 {
			return 0, 0, false
		}
		return i, i + len(kw), true
	}

	// Length-changing folds (e.g. İ → i + combining dot): map each lowered byte
	// back to the original rune span that produced it.
	lower, origStart, origEnd := lowerWithOrigMap(s)
	i := strings.Index(lower, kw)
	if i < 0 {
		return 0, 0, false
	}
	j := i + len(kw) - 1
	if j >= len(origEnd) {
		return 0, 0, false
	}
	return origStart[i], origEnd[j], true
}

// lowerWithOrigMap returns strings.ToLower(s) and parallel slices:
// for each byte j of the lowered string, origStart[j]/origEnd[j] are the
// [start,end) byte range of the original rune that produced that lowered byte.
func lowerWithOrigMap(s string) (lower string, origStart, origEnd []int) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		_, size := utf8.DecodeRuneInString(s[i:])
		if size < 1 {
			size = 1
		}
		low := strings.ToLower(s[i : i+size])
		for range low {
			origStart = append(origStart, i)
			origEnd = append(origEnd, i+size)
		}
		b.WriteString(low)
		i += size
	}
	return b.String(), origStart, origEnd
}

// WriteMatch prints one hit using a plain (no-color) Printer.
// Prefer constructing a Printer when formatting many matches.
// Always Flushes so context matches are not left buffered.
func WriteMatch(w io.Writer, m searcher.Match) error {
	p := &Printer{NoColor: true}
	if err := p.WriteMatch(w, m); err != nil {
		return err
	}
	return p.Flush(w)
}
