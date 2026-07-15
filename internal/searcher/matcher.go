package searcher

import (
	"fmt"
	"regexp"
	"strings"
)

// matcher decides whether a line is a hit for the configured keyword/pattern.
// Highlight spans live in internal/output (separate from search matching).
type matcher interface {
	match(line string) bool
}

// newMatcher builds a literal or regex matcher.
// keyword must be non-empty. When regex is true, keyword is a Go RE2 pattern;
// invalid patterns return an error wrapping the compile failure.
func newMatcher(keyword string, ignoreCase, regex bool) (matcher, error) {
	if keyword == "" {
		return nil, fmt.Errorf("searcher: keyword is required")
	}
	if !regex {
		kw := keyword
		if ignoreCase {
			kw = strings.ToLower(keyword)
		}
		return literalMatcher{keyword: kw, ignoreCase: ignoreCase}, nil
	}

	pattern := keyword
	if ignoreCase {
		// RE2 inline flag: case-insensitive for the whole pattern.
		pattern = "(?i)" + keyword
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("searcher: invalid regex: %w", err)
	}
	return regexMatcher{re: re}, nil
}

// literalMatcher uses strings.Contains (case-sensitive or lowercased).
// keyword is already lowercased when ignoreCase is true.
type literalMatcher struct {
	keyword    string
	ignoreCase bool
}

func (m literalMatcher) match(line string) bool {
	return lineMatch(line, m.keyword, m.ignoreCase)
}

// regexMatcher uses a compiled RE2 pattern.
type regexMatcher struct {
	re *regexp.Regexp
}

func (m regexMatcher) match(line string) bool {
	return m.re.MatchString(line)
}

// lineMatch reports whether line contains keyword.
// When ignoreCase is true, keyword must already be lowercased by the caller.
func lineMatch(line, keyword string, ignoreCase bool) bool {
	if !ignoreCase {
		return strings.Contains(line, keyword)
	}
	return strings.Contains(strings.ToLower(line), keyword)
}
