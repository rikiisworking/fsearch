package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nick/fsearch/internal/searcher"
)

func TestJSONPrinterNoContext(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONPrinter{}
	m := searcher.Match{Path: "main.go", Line: 3, Content: "TODO fix me"}
	if err := p.WriteMatch(&buf, m); err != nil {
		t.Fatalf("WriteMatch: %v", err)
	}
	if err := p.Flush(&buf); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", line, err)
	}
	if got["path"] != "main.go" || got["line"] != float64(3) || got["content"] != "TODO fix me" {
		t.Errorf("got %#v", got)
	}
	if _, ok := got["before"]; ok {
		t.Errorf("before should be omitted, got %#v", got["before"])
	}
	if _, ok := got["after"]; ok {
		t.Errorf("after should be omitted, got %#v", got["after"])
	}
	// Exactly one NDJSON line (Encode adds trailing newline).
	if strings.Count(buf.String(), "\n") != 1 {
		t.Errorf("want single trailing newline, got %q", buf.String())
	}
}

func TestJSONPrinterWithContext(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONPrinter{}
	m := searcher.Match{
		Path:    "a.txt",
		Line:    2,
		Content: "HIT",
		Before:  []string{"before"},
		After:   []string{"after"},
	}
	if err := p.WriteMatch(&buf, m); err != nil {
		t.Fatalf("WriteMatch: %v", err)
	}

	var got jsonMatch
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Path != "a.txt" || got.Line != 2 || got.Content != "HIT" {
		t.Errorf("got %+v", got)
	}
	if len(got.Before) != 1 || got.Before[0] != "before" {
		t.Errorf("Before = %v", got.Before)
	}
	if len(got.After) != 1 || got.After[0] != "after" {
		t.Errorf("After = %v", got.After)
	}
}

func TestJSONPrinterMultipleLines(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONPrinter{}
	ms := []searcher.Match{
		{Path: "a.go", Line: 1, Content: "one"},
		{Path: "b.go", Line: 2, Content: "two"},
	}
	for _, m := range ms {
		if err := p.WriteMatch(&buf, m); err != nil {
			t.Fatalf("WriteMatch: %v", err)
		}
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), buf.String())
	}
	for i, line := range lines {
		var got jsonMatch
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if got.Content != ms[i].Content {
			t.Errorf("line %d content = %q, want %q", i, got.Content, ms[i].Content)
		}
	}
}

func TestJSONPrinterNoANSI(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONPrinter{}
	m := searcher.Match{Path: "x", Line: 1, Content: "TODO"}
	if err := p.WriteMatch(&buf, m); err != nil {
		t.Fatalf("WriteMatch: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte{0x1b}) {
		t.Errorf("unexpected ANSI in JSON: %q", buf.String())
	}
}

func TestJSONPrinterNoHTMLEscape(t *testing.T) {
	// Paths/content with <>& should not become \u003c etc.
	var buf bytes.Buffer
	p := &JSONPrinter{}
	m := searcher.Match{Path: "a<b>.go", Line: 1, Content: "x & y"}
	if err := p.WriteMatch(&buf, m); err != nil {
		t.Fatalf("WriteMatch: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, `"path":"a<b>.go"`) {
		t.Errorf("path should not be HTML-escaped: %q", s)
	}
	if !strings.Contains(s, `"content":"x & y"`) {
		t.Errorf("content should not be HTML-escaped: %q", s)
	}
}
