package searcher

import (
	"fmt"
	"regexp"
	"strings"
)

// matcher decides whether a line is a hit and where spans fall (for highlight).
type matcher interface {
	match(line string) bool
	// findAll returns non-overlapping [start,end) byte spans in line.
	findAll(line string) [][]int
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

func (m literalMatcher) findAll(line string) [][]int {
	if m.keyword == "" {
		return nil
	}
	var spans [][]int
	if !m.ignoreCase {
		start := 0
		for {
			i := strings.Index(line[start:], m.keyword)
			if i < 0 {
				break
			}
			abs := start + i
			end := abs + len(m.keyword)
			spans = append(spans, []int{abs, end})
			start = end
		}
		return spans
	}

	// Case-insensitive: search in lowered text when lengths match (common path).
	lower := strings.ToLower(line)
	kw := m.keyword // already lower
	if len(lower) != len(line) {
		// Length-changing folds: precise multi-span mapping is rare; skip spans
		// rather than return corrupt byte offsets (match() still works).
		return nil
	}
	start := 0
	for {
		i := strings.Index(lower[start:], kw)
		if i < 0 {
			break
		}
		abs := start + i
		end := abs + len(kw)
		spans = append(spans, []int{abs, end})
		start = end
	}
	return spans
}

// regexMatcher uses a compiled RE2 pattern.
type regexMatcher struct {
	re *regexp.Regexp
}

func (m regexMatcher) match(line string) bool {
	return m.re.MatchString(line)
}

func (m regexMatcher) findAll(line string) [][]int {
	return m.re.FindAllStringIndex(line, -1)
}
