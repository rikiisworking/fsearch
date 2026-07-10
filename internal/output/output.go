// Package output formats and prints search results for the terminal.
package output

import (
	"fmt"
	"io"
)

// WriteMatch prints one hit in grep-style format: path:line:content
func WriteMatch(w io.Writer, path string, line int, content string) error {
	_, err := fmt.Fprintf(w, "%s:%d:%s\n", path, line, content)
	if err != nil {
		return fmt.Errorf("output: write match: %w", err)
	}
	return nil
}
