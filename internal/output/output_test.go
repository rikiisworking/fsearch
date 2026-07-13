package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"

	"github.com/nick/fsearch/internal/searcher"
)

func TestWriteMatchNoContext(t *testing.T) {
	var buf bytes.Buffer
	m := searcher.Match{
		Path:    "main.go",
		Line:    3,
		Content: "TODO fix me",
	}
	if err := WriteMatch(&buf, m); err != nil {
		t.Fatalf("WriteMatch: %v", err)
	}
	want := "main.go:3:TODO fix me\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrinterContext(t *testing.T) {
	p := &Printer{NoColor: true}
	var buf bytes.Buffer

	m := searcher.Match{
		Path:    "a.txt",
		Line:    3,
		Content: "HIT one",
		Before:  []string{"beta"},
		After:   []string{"gamma"},
	}
	if err := p.WriteMatch(&buf, m); err != nil {
		t.Fatalf("WriteMatch: %v", err)
	}

	want := "" +
		"a.txt-2-beta\n" +
		"a.txt:3:HIT one\n" +
		"a.txt-4-gamma\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrinterContextSeparator(t *testing.T) {
	p := &Printer{NoColor: true}
	var buf bytes.Buffer

	m1 := searcher.Match{
		Path:    "a.txt",
		Line:    3,
		Content: "HIT one",
		Before:  []string{"beta"},
		After:   []string{"gamma"},
	}
	m2 := searcher.Match{
		Path:    "a.txt",
		Line:    5,
		Content: "HIT two",
		Before:  []string{"gamma"},
		After:   []string{"delta"},
	}

	if err := p.WriteMatch(&buf, m1); err != nil {
		t.Fatalf("WriteMatch m1: %v", err)
	}
	if err := p.WriteMatch(&buf, m2); err != nil {
		t.Fatalf("WriteMatch m2: %v", err)
	}

	want := "" +
		"a.txt-2-beta\n" +
		"a.txt:3:HIT one\n" +
		"a.txt-4-gamma\n" +
		"--\n" +
		"a.txt-4-gamma\n" +
		"a.txt:5:HIT two\n" +
		"a.txt-6-delta\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrinterNoContextNoSeparator(t *testing.T) {
	p := &Printer{NoColor: true}
	var buf bytes.Buffer

	m1 := searcher.Match{Path: "a.txt", Line: 1, Content: "HIT one"}
	m2 := searcher.Match{Path: "a.txt", Line: 2, Content: "HIT two"}

	if err := p.WriteMatch(&buf, m1); err != nil {
		t.Fatalf("WriteMatch m1: %v", err)
	}
	if err := p.WriteMatch(&buf, m2); err != nil {
		t.Fatalf("WriteMatch m2: %v", err)
	}

	want := "a.txt:1:HIT one\na.txt:2:HIT two\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrinterContextEdges(t *testing.T) {
	// First-line hit: empty Before; last-line hit: empty After.
	p := &Printer{NoColor: true}
	var buf bytes.Buffer

	first := searcher.Match{
		Path:    "e.txt",
		Line:    1,
		Content: "HIT first",
		After:   []string{"middle"},
	}
	last := searcher.Match{
		Path:    "e.txt",
		Line:    3,
		Content: "HIT last",
		Before:  []string{"middle"},
	}

	if err := p.WriteMatch(&buf, first); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := p.WriteMatch(&buf, last); err != nil {
		t.Fatalf("last: %v", err)
	}

	want := "" +
		"e.txt:1:HIT first\n" +
		"e.txt-2-middle\n" +
		"--\n" +
		"e.txt-2-middle\n" +
		"e.txt:3:HIT last\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrinterNoColorFlag(t *testing.T) {
	// Even if the global color package would emit ANSI, NoColor must force plain text.
	p := &Printer{NoColor: true, Keyword: "TODO"}
	var buf bytes.Buffer
	m := searcher.Match{Path: "main.go", Line: 3, Content: "TODO fix me"}
	if err := p.WriteMatch(&buf, m); err != nil {
		t.Fatalf("WriteMatch: %v", err)
	}
	got := buf.String()
	want := "main.go:3:TODO fix me\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if bytes.Contains(buf.Bytes(), []byte{0x1b}) {
		t.Errorf("unexpected ANSI codes in no-color output: %q", got)
	}
}

func TestHighlight(t *testing.T) {
	// Build expected colored spans the same way highlight does (stable across fatih/color).
	paint := func(s string) string {
		c := color.New(color.FgRed, color.Bold)
		c.EnableColor()
		return c.Sprint(s)
	}

	tests := []struct {
		name       string
		content    string
		keyword    string
		ignoreCase bool
		enabled    bool
		want       string
	}{
		{
			name:    "disabled returns plain",
			content: "TODO fix TODO",
			keyword: "TODO",
			enabled: false,
			want:    "TODO fix TODO",
		},
		{
			name:    "empty keyword",
			content: "TODO",
			keyword: "",
			enabled: true,
			want:    "TODO",
		},
		{
			name:    "no match",
			content: "hello",
			keyword: "TODO",
			enabled: true,
			want:    "hello",
		},
		{
			name:    "single match case-sensitive",
			content: "xx TODOyy",
			keyword: "TODO",
			enabled: true,
			want:    "xx " + paint("TODO") + "yy",
		},
		{
			name:    "multiple matches",
			content: "TODO and TODO",
			keyword: "TODO",
			enabled: true,
			want:    paint("TODO") + " and " + paint("TODO"),
		},
		{
			name:       "ignore-case preserves original casing",
			content:    "todo ToDo TODO",
			keyword:    "TODO",
			ignoreCase: true,
			enabled:    true,
			want:       paint("todo") + " " + paint("ToDo") + " " + paint("TODO"),
		},
		{
			name:    "case-sensitive skips wrong case",
			content: "todo TODO",
			keyword: "TODO",
			enabled: true,
			want:    "todo " + paint("TODO"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := highlight(tt.content, tt.keyword, tt.ignoreCase, tt.enabled)
			if got != tt.want {
				t.Errorf("highlight() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrinterHighlightOnlyOnHitLine(t *testing.T) {
	// Force color on for this test regardless of TTY.
	p := &Printer{Keyword: "HIT", NoColor: false}
	old := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = old })

	var buf bytes.Buffer
	m := searcher.Match{
		Path:    "a.txt",
		Line:    2,
		Content: "HIT mid",
		Before:  []string{"before HIT"},
		After:   []string{"after HIT"},
	}
	if err := p.WriteMatch(&buf, m); err != nil {
		t.Fatalf("WriteMatch: %v", err)
	}
	got := buf.String()

	painted := func(s string) string {
		c := color.New(color.FgRed, color.Bold)
		c.EnableColor()
		return c.Sprint(s)
	}("HIT")

	// Hit content should wrap HIT; context lines should not.
	if !strings.Contains(got, painted+" mid") {
		t.Errorf("hit line missing keyword highlight: %q", got)
	}
	if strings.Contains(got, "before "+painted) || strings.Contains(got, "after "+painted) {
		t.Errorf("context lines should not highlight keyword: %q", got)
	}
	// Plain text still present in context.
	if !strings.Contains(got, "before HIT") || !strings.Contains(got, "after HIT") {
		t.Errorf("context plain keyword missing: %q", got)
	}
}
