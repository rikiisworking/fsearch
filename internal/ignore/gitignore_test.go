package ignore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseGitignore(t *testing.T) {
	const content = `
# comment
*.o
/build
logs/
!important.log

  # spaced comment
`

	got := ParseGitignore(content)
	want := []Rule{
		{Pattern: "*.o"},
		{Pattern: "build", Anchored: true},
		{Pattern: "logs", DirOnly: true},
		{Pattern: "important.log", Negate: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseGitignore() = %#v, want %#v", got, want)
	}
}

func TestParseGitignoreLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		ok   bool
		want Rule
	}{
		{"empty", "", false, Rule{}},
		{"spaces", "   ", false, Rule{}},
		{"comment", "# hi", false, Rule{}},
		{"plain", "foo", true, Rule{Pattern: "foo"}},
		{"glob", "*.log", true, Rule{Pattern: "*.log"}},
		{"negate", "!bar", true, Rule{Pattern: "bar", Negate: true}},
		{"dir only", "tmp/", true, Rule{Pattern: "tmp", DirOnly: true}},
		{"anchored", "/root-only", true, Rule{Pattern: "root-only", Anchored: true}},
		{"anchored dir", "/out/", true, Rule{Pattern: "out", Anchored: true, DirOnly: true}},
		{"bang only", "!", false, Rule{}},
		{"slash only", "/", false, Rule{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseGitignoreLine(tt.line)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (got %#v)", ok, tt.ok, got)
			}
			if ok && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rule = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRuleMatch(t *testing.T) {
	tests := []struct {
		name  string
		rule  Rule
		path  string
		isDir bool
		want  bool
	}{
		{"glob file", Rule{Pattern: "*.o"}, "a.o", false, true},
		{"glob nested component", Rule{Pattern: "*.o"}, "src/a.o", false, true},
		{"glob miss", Rule{Pattern: "*.o"}, "a.c", false, false},

		{"unanchored name anywhere", Rule{Pattern: "build"}, "pkg/build", true, true},
		{"unanchored root name", Rule{Pattern: "build"}, "build", true, true},

		{"anchored hit", Rule{Pattern: "build", Anchored: true}, "build", true, true},
		{"anchored miss nested", Rule{Pattern: "build", Anchored: true}, "pkg/build", true, false},
		{"anchored prefix child", Rule{Pattern: "build", Anchored: true}, "build/x.go", false, true},

		{"dir only on file", Rule{Pattern: "tmp", DirOnly: true}, "tmp", false, false},
		{"dir only on dir", Rule{Pattern: "tmp", DirOnly: true}, "tmp", true, true},

		{"slash pattern", Rule{Pattern: "a/b"}, "a/b", false, true},
		{"slash pattern miss", Rule{Pattern: "a/b"}, "a/c", false, false},

		{"empty path", Rule{Pattern: "x"}, "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.Match(tt.path, tt.isDir); got != tt.want {
				t.Errorf("Match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestGitignoreMatchLastWins(t *testing.T) {
	// *.log ignored, then !important.log un-ignored.
	g := NewGitignore(ParseGitignore("*.log\n!important.log\n"))

	tests := []struct {
		path string
		want bool // true = ignored
	}{
		{"debug.log", true},
		{"dir/foo.log", true},
		{"important.log", false},
		{"readme.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := g.Match(tt.path, false); got != tt.want {
				t.Errorf("Match(%q) ignored=%v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestGitignoreMatchOrder(t *testing.T) {
	// Later rule overrides earlier: ignore foo, then ignore nothing via re-ignore.
	// !foo after foo → not ignored.
	g := NewGitignore([]Rule{
		{Pattern: "foo"},
		{Pattern: "foo", Negate: true},
	})
	if g.Match("foo", false) {
		t.Fatal("expected foo not ignored after negate")
	}

	// Opposite order: un-ignore then ignore → ignored.
	g2 := NewGitignore([]Rule{
		{Pattern: "foo", Negate: true},
		{Pattern: "foo"},
	})
	if !g2.Match("foo", false) {
		t.Fatal("expected foo ignored when ignore comes last")
	}
}

func TestGitignoreNilEmpty(t *testing.T) {
	if (*Gitignore)(nil).Match("x", false) {
		t.Fatal("nil Gitignore should not ignore")
	}
	if NewGitignore(nil).Match("x", false) {
		t.Fatal("empty Gitignore should not ignore")
	}
}

func TestLoadGitignoreFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	content := "*.log\n!keep.log\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	g, err := LoadGitignoreFile(path)
	if err != nil {
		t.Fatalf("LoadGitignoreFile: %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil Gitignore")
	}
	if !g.Match("a.log", false) {
		t.Error("expected a.log ignored")
	}
	if g.Match("keep.log", false) {
		t.Error("expected keep.log not ignored")
	}
}

func TestLoadGitignoreFileMissing(t *testing.T) {
	g, err := LoadGitignoreFile(filepath.Join(t.TempDir(), ".gitignore"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if g != nil {
		t.Fatalf("missing file want nil Gitignore, got %#v", g)
	}
}
