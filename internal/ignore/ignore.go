// Package ignore decides which paths to skip during a walk (patterns, defaults).
package ignore

import (
	"path/filepath"
	"strings"
)

// Matcher decides which directories to prune and which files to search.
type Matcher struct {
	includeExts  map[string]struct{} // allow-list; empty = all extensions
	skipPatterns []string            // basename patterns to skip
	// Optional root-.gitignore rules (path-relative to root).
	root string
	git  *Gitignore
}

// New builds a Matcher from an extension allow-list and skip patterns.
// Extensions may include or omit a leading dot (e.g. "go" or ".go").
// Empty allowedExts means all extensions are allowed.
func New(allowedExts []string, skipPatterns []string) *Matcher {
	m := &Matcher{
		includeExts:  make(map[string]struct{}),
		skipPatterns: append([]string(nil), skipPatterns...),
	}
	for _, e := range allowedExts {
		e = strings.TrimSpace(e)
		e = strings.TrimPrefix(e, ".")
		if e == "" {
			continue
		}
		m.includeExts[strings.ToLower(e)] = struct{}{}
	}
	return m
}

// SetGitignore attaches rules from a root .gitignore.
// root is the walk/search root (paths are matched relative to it).
// g may be nil to clear gitignore rules.
func (m *Matcher) SetGitignore(root string, g *Gitignore) {
	if m == nil {
		return
	}
	m.root = root
	m.git = g
}

// rel returns path relative to m.root for gitignore matching.
func (m *Matcher) rel(path string) string {
	if m.root == "" {
		return path
	}
	rel, err := filepath.Rel(m.root, path)
	if err != nil {
		return path
	}
	return rel
}

// defaultSkipDirs are directories pruned during a walk unless overridden later.
var defaultSkipDirs = map[string]struct{}{
	// VCS
	".git": {},
	".hg":  {},
	".svn": {},
	".bzr": {},

	// Dependencies
	"node_modules":     {},
	"vendor":           {},
	"bower_components": {},
	".bundle":          {},

	// Build / output
	"bin":      {},
	"dist":     {},
	"build":    {},
	"target":   {},
	"out":      {},
	"_build":   {},
	"coverage": {},

	// Language / tool caches
	"__pycache__":   {},
	".mypy_cache":   {},
	".pytest_cache": {},
	".ruff_cache":   {},
	".tox":          {},
	".nox":          {},
	".cache":        {},

	// Virtual envs (avoid bare "env" — too common as source name)
	".venv": {},
	"venv":  {},

	// JS / framework
	".next":         {},
	".nuxt":         {},
	".turbo":        {},
	".parcel-cache": {},

	// Other tool state
	".gradle":    {},
	".terraform": {},
	".eggs":      {},
	".devenv":    {},
	".direnv":    {},

	// IDE / editor
	".idea":   {},
	".vscode": {},
	".vs":     {},
}

// SkipDir reports whether a directory should be pruned.
// path is the full directory path; name is its basename.
// Order: built-in defaults and --ignore patterns (basename), then .gitignore.
func (m *Matcher) SkipDir(path, name string) bool {
	if name == "" {
		return false
	}
	if _, ok := defaultSkipDirs[name]; ok {
		return true
	}
	if m.matchesSkipPattern(name) {
		return true
	}
	if m.git != nil && m.git.Match(m.rel(path), true) {
		return true
	}
	return false
}

// IncludeFile reports whether the file at path should be searched.
// Order: --ignore basename patterns, .gitignore, then extension allow-list.
func (m *Matcher) IncludeFile(path string) bool {
	base := filepath.Base(path)
	if m.matchesSkipPattern(base) {
		return false
	}
	if m.git != nil && m.git.Match(m.rel(path), false) {
		return false
	}
	ext := strings.TrimPrefix(filepath.Ext(base), ".")
	if len(m.includeExts) == 0 {
		return true
	}
	if ext == "" {
		return false // e.g. Makefile — no extension, filtered mode
	}
	_, ok := m.includeExts[strings.ToLower(ext)]
	return ok
}

// matchesSkipPattern reports whether name matches any user skip pattern.
// Supports exact basename match and path.Match globs (e.g. "tmp*", "*.cache").
func (m *Matcher) matchesSkipPattern(name string) bool {
	for _, p := range m.skipPatterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == name {
			return true
		}
		if ok, err := filepath.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}
