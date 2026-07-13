package output

import (
	"bytes"
	"testing"

	"github.com/nick/fsearch/internal/searcher"
)

func TestWriteMatch(t *testing.T) {
	var buf bytes.Buffer
	m := searcher.Match{
		Path:    "main.go",
		Line:    3,
		Content: "TODO fix me",
	}
	if err := WriteMatch(&buf, m); err != nil {
		t.Errorf("WriteMatch: %v", err)
		return
	}
	want := "main.go:3:TODO fix me\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
