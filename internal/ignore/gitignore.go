package ignore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Rule is one parsed .gitignore pattern (subset of git's rules).
//
// Supported line forms:
//   - blank lines and # comments are ignored
//   - leading !  → Negate (un-ignore); Negate is stored for list matching (later)
//   - trailing / → DirOnly (directories only)
//   - leading /  → Anchored (match relative to the .gitignore root only)
type Rule struct {
	// Pattern is the path pattern with leading ! and / and trailing / removed.
	Pattern string
	Negate  bool
	DirOnly bool
	// Anchored means the pattern was rooted with a leading slash.
	Anchored bool
}

// Match reports whether path matches this rule's pattern (ignores Negate).
// path is relative to the .gitignore root and should use slashes (or will be normalized).
// isDir is true when path names a directory.
//
// MVP semantics:
//   - DirOnly rules never match non-directories
//   - Anchored rules, or patterns containing "/", match against the full path
//     (and as a directory prefix: pattern "build" matches "build/x")
//   - Other rules match any single path component (e.g. "*.o" matches "src/a.o")
func (r Rule) Match(path string, isDir bool) bool {
	if r.Pattern == "" {
		return false
	}
	if r.DirOnly && !isDir {
		return false
	}
	path = normalizeGitPath(path)
	if path == "" {
		return false
	}

	if r.Anchored || strings.Contains(r.Pattern, "/") {
		return matchFullPath(r.Pattern, path)
	}
	for _, part := range strings.Split(path, "/") {
		if matchName(r.Pattern, part) {
			return true
		}
	}
	return false
}

func normalizeGitPath(path string) string {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	return strings.Trim(path, "/")
}

// matchFullPath matches pattern against the full relative path, or as a
// directory prefix for literal patterns (pattern "out" matches "out" and "out/x").
// Glob patterns only match the full path via filepath.Match (MVP: no glob prefix walk).
func matchFullPath(pattern, path string) bool {
	if matchName(pattern, path) {
		return true
	}
	// Literal directory prefix only (unsafe for globs like "out*").
	if !strings.ContainsAny(pattern, "*?[]") && strings.HasPrefix(path, pattern+"/") {
		return true
	}
	return false
}

func matchName(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}

// Gitignore is an ordered list of rules (as in a .gitignore file).
type Gitignore struct {
	rules []Rule
}

// NewGitignore wraps rules for matching (typically from ParseGitignore).
func NewGitignore(rules []Rule) *Gitignore {
	// Copy so callers cannot mutate our slice unexpectedly.
	cp := append([]Rule(nil), rules...)
	return &Gitignore{rules: cp}
}

// Match reports whether path should be ignored under gitignore last-match-wins
// semantics (including negation). nil or empty rules → not ignored.
//
// Walks rules in order; each match sets ignored = !rule.Negate. The final
// value after the last matching rule wins.
func (g *Gitignore) Match(path string, isDir bool) bool {
	if g == nil || len(g.rules) == 0 {
		return false
	}
	ignored := false
	for _, r := range g.rules {
		if !r.Match(path, isDir) {
			continue
		}
		// Pattern hit: ignore unless this rule is a negation (!pattern).
		ignored = !r.Negate
	}
	return ignored
}

// LoadGitignoreFile reads path, parses it as a .gitignore, and returns a
// Gitignore matcher. If the file does not exist, returns (nil, nil) so callers
// can treat a missing file as "no gitignore rules".
// Other I/O errors are wrapped and returned.
func LoadGitignoreFile(path string) (*Gitignore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("ignore: read gitignore %s: %w", path, err)
	}
	return NewGitignore(ParseGitignore(string(data))), nil
}

// ParseGitignore parses .gitignore file content into rules (in file order).
// Empty patterns after cleaning are skipped.
func ParseGitignore(content string) []Rule {
	var rules []Rule
	for _, line := range strings.Split(content, "\n") {
		if r, ok := parseGitignoreLine(line); ok {
			rules = append(rules, r)
		}
	}
	return rules
}

// parseGitignoreLine parses a single line. ok is false for blanks/comments/empty.
func parseGitignoreLine(line string) (Rule, bool) {
	// MVP: trim spaces (git has finer rules for escaped trailing spaces).
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return Rule{}, false
	}

	var r Rule
	if strings.HasPrefix(line, "!") {
		r.Negate = true
		line = strings.TrimPrefix(line, "!")
	}
	if line == "" {
		return Rule{}, false
	}

	if strings.HasPrefix(line, "/") {
		r.Anchored = true
		line = strings.TrimPrefix(line, "/")
	}
	if strings.HasSuffix(line, "/") {
		r.DirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if line == "" {
		return Rule{}, false
	}

	r.Pattern = line
	return r, true
}
