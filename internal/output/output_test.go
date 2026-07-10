package output

import (
	"bytes"
	"testing"
)

func TestWriteMatch(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMatch(&buf, "main.go", 3, "TODO fix me"); err != nil {
		t.Errorf("WriteMatch: %v", err)
		return
	}
	want := "main.go:3:TODO fix me\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
