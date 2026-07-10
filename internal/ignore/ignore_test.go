package ignore

import "testing"

func TestSkipDirDefaults(t *testing.T) {
	// No custom patterns — only built-in defaultSkipDirs.
	m := New(nil, nil)

	tests := []struct {
		name string
		want bool
	}{
		// should skip (sample of aggressive defaults)
		{".git", true},
		{"node_modules", true},
		{"vendor", true},
		{"__pycache__", true},
		{".venv", true},
		{".idea", true},
		{"dist", true},
		{"target", true},
		{".next", true},

		// should not skip
		{"src", false},
		{"internal", false},
		{"cmd", false},
		{"", false},
		{"my.git", false}, // not exact ".git"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.SkipDir(tt.name); got != tt.want {
				t.Errorf("SkipDir(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestSkipDirPatterns(t *testing.T) {
	m := New(nil, []string{"build", "tmp*", "secret"})

	tests := []struct {
		name string // subtest label
		dir  string // basename passed to SkipDir
		want bool
	}{
		// custom exact
		{"exact build", "build", true},
		{"exact secret", "secret", true},

		// custom glob tmp*
		{"glob tmp", "tmp", true},
		{"glob tmp-cache", "tmp-cache", true},

		// no match on custom
		{"src allowed", "src", false},
		{"build-output no exact", "build-output", false}, // pattern is "build", not "build*"
		{"mysecret no substring", "mysecret", false},

		// defaults still work with patterns present
		{"default .git", ".git", true},
		{"default node_modules", "node_modules", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.SkipDir(tt.dir); got != tt.want {
				t.Errorf("SkipDir(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

func TestIncludeFile(t *testing.T) {
	tests := []struct {
		name         string
		allowedExts  []string
		skipPatterns []string
		path         string
		want         bool
	}{
		// no ext filter → allow all (unless skip pattern)
		{"allow all txt", nil, nil, "foo/bar.txt", true},
		{"allow all go", nil, nil, "pkg/main.go", true},
		{"allow Makefile no ext", nil, nil, "Makefile", true},

		// extension allow-list
		{"ext go match", []string{"go"}, nil, "pkg/main.go", true},
		{"ext go with dot", []string{".go", "md"}, nil, "docs/README.md", true},
		{"ext reject py", []string{"go"}, nil, "script.py", false},
		{"ext reject no ext", []string{"go"}, nil, "Makefile", false},
		{"ext case insensitive", []string{"GO"}, nil, "pkg/X.Go", true},

		// path-shaped: only basename/ext matter
		{"nested path ext", []string{"go"}, nil, "a/b/c/d.go", true},
		{"nested path reject", []string{"go"}, nil, "a/b/c/d.py", false},

		// skip patterns on files
		{"skip exact basename", nil, []string{"secret.env"}, "cfg/secret.env", false},
		{"skip glob min.js", nil, []string{"*.min.js"}, "static/app.min.js", false},
		{"skip glob allows other js", nil, []string{"*.min.js"}, "static/app.js", true},

		// skip pattern wins even if ext would allow
		{"skip wins over ext", []string{"js"}, []string{"*.min.js"}, "app.min.js", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.allowedExts, tt.skipPatterns)
			if got := m.IncludeFile(tt.path); got != tt.want {
				t.Errorf("IncludeFile(%q) = %v, want %v (allowedExts=%v skipPatterns=%v)",
					tt.path, got, tt.want, tt.allowedExts, tt.skipPatterns)
			}
		})
	}
}
