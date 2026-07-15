package searcher

import (
	"strings"
	"testing"
)

func TestNewMatcherEmptyKeyword(t *testing.T) {
	_, err := newMatcher("", false, false)
	if err == nil {
		t.Fatal("expected error for empty keyword")
	}
	if !strings.Contains(err.Error(), "keyword is required") {
		t.Errorf("error = %q, want keyword is required", err)
	}
}

func TestNewMatcherInvalidRegex(t *testing.T) {
	_, err := newMatcher("(", false, true)
	if err == nil {
		t.Fatal("expected invalid regex error")
	}
	if !strings.Contains(err.Error(), "invalid regex") {
		t.Errorf("error = %q, want invalid regex", err)
	}
}

func TestMatcherMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		keyword    string
		ignoreCase bool
		regex      bool
		line       string
		wantMatch  bool
	}{
		{
			name:      "literal hit",
			keyword:   "TODO",
			line:      "// TODO here",
			wantMatch: true,
		},
		{
			name:      "literal miss",
			keyword:   "TODO",
			line:      "// done",
			wantMatch: false,
		},
		{
			name:      "literal case sensitive miss",
			keyword:   "Todo",
			line:      "// TODO",
			wantMatch: false,
		},
		{
			name:       "literal ignore case hit",
			keyword:    "todo",
			ignoreCase: true,
			line:       "// TODO here",
			wantMatch:  true,
		},
		{
			name:       "literal ignore case miss",
			keyword:    "todo",
			ignoreCase: true,
			line:       "// done",
			wantMatch:  false,
		},
		{
			name:      "regex alternation hit",
			keyword:   `TODO|FIXME`,
			regex:     true,
			line:      "// FIXME x",
			wantMatch: true,
		},
		{
			name:      "regex anchored miss",
			keyword:   `^TODO`,
			regex:     true,
			line:      "  TODO",
			wantMatch: false,
		},
		{
			name:       "regex ignore case hit",
			keyword:    `todo`,
			ignoreCase: true,
			regex:      true,
			line:       "// TODO",
			wantMatch:  true,
		},
		{
			name:      "regex quantifier",
			keyword:   `a+`,
			regex:     true,
			line:      "xaaay",
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, err := newMatcher(tt.keyword, tt.ignoreCase, tt.regex)
			if err != nil {
				t.Fatalf("newMatcher: %v", err)
			}
			if got := m.match(tt.line); got != tt.wantMatch {
				t.Errorf("match(%q) = %v, want %v", tt.line, got, tt.wantMatch)
			}
		})
	}
}
