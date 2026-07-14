package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nick/fsearch/internal/searcher"
)

func TestParseList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"spaces only", "  ", nil},
		{"single", "go", []string{"go"}},
		{"comma list", "go,md", []string{"go", "md"}},
		{"spaces around", " go , md ", []string{"go", "md"}},
		{"only commas", ",,", nil},
		{"trailing comma", "go,", []string{"go"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseList(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseList(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildOptions(t *testing.T) {
	tests := []struct {
		name         string
		keyword      string
		root         string
		exts         string
		ignores      []string
		ignoreCase   bool
		contextLines int
		wantOpts     searcher.Options
	}{
		{
			name:    "defaults",
			keyword: "TODO",
			root:    ".",
			wantOpts: searcher.Options{
				Root:    ".",
				Keyword: "TODO",
			},
		},
		{
			name:    "exts and single ignore",
			keyword: "FIXME",
			root:    "./internal",
			exts:    "go,md",
			ignores: []string{"vendor"},
			wantOpts: searcher.Options{
				Root:         "./internal",
				Keyword:      "FIXME",
				AllowedExts:  []string{"go", "md"},
				SkipPatterns: []string{"vendor"},
			},
		},
		{
			name:    "repeatable ignore and comma list",
			keyword: "TODO",
			root:    ".",
			ignores: []string{"a", "b,c"},
			wantOpts: searcher.Options{
				Root:         ".",
				Keyword:      "TODO",
				SkipPatterns: []string{"a", "b", "c"},
			},
		},
		{
			name:       "ignore-case",
			keyword:    "todo",
			root:       ".",
			ignoreCase: true,
			wantOpts: searcher.Options{
				Root:       ".",
				Keyword:    "todo",
				IgnoreCase: true,
			},
		},
		{
			name:         "context lines",
			keyword:      "TODO",
			root:         ".",
			contextLines: 2,
			wantOpts: searcher.Options{
				Root:         ".",
				Keyword:      "TODO",
				ContextLines: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildOptions(tt.keyword, tt.root, tt.exts, tt.ignores, tt.ignoreCase, tt.contextLines)
			if !reflect.DeepEqual(got, tt.wantOpts) {
				t.Errorf("buildOptions() = %#v, want %#v", got, tt.wantOpts)
			}
		})
	}
}

func TestCLISmokeHit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a\n// TODO here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nout=%q", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "TODO here") {
		t.Errorf("output missing hit content: %q", got)
	}
	if !strings.Contains(got, path) && !strings.Contains(got, "a.go") {
		t.Errorf("output missing path: %q", got)
	}
	// grep-style: path:line:content
	if !strings.Contains(got, ":") {
		t.Errorf("expected path:line:content format, got %q", got)
	}
}

func TestCLISmokeExtFilter(t *testing.T) {
	root := t.TempDir()
	// Keyword only in .md — filtered out by --ext go.
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("# TODO only md\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("package b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nout=%q", err, out.String())
	}
	if got := out.String(); got != "" {
		t.Errorf("want empty output for filtered miss, got %q", got)
	}
}

func TestCLISmokeMissingArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs(nil)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for missing args")
	}
}

func TestCLISmokeIgnoreCase(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a\n// TODO here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Case-sensitive (default): lowercase keyword misses TODO
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"todo", root, "--ext", "go"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute (sensitive): %v\nout=%q", err, out.String())
	}
	if got := out.String(); got != "" {
		t.Errorf("case-sensitive want empty, got %q", got)
	}

	// Ignore-case: should hit
	out.Reset()
	cmd = newRootCmd()
	cmd.SetArgs([]string{"todo", root, "--ext", "go", "-i"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute (-i): %v\nout=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "TODO here") {
		t.Errorf("-i output missing hit: %q", out.String())
	}
}

func TestCLISmokeContext(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	// line1 package, line2 blank-ish comment prev, line3 TODO, line4 next
	content := "package a\n// prev\n// TODO here\n// next\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go", "-C", "1", "--no-color"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nout=%q", err, out.String())
	}

	got := out.String()
	// Hit line uses :
	if !strings.Contains(got, ":3:// TODO here") && !strings.Contains(got, path+":3:// TODO here") {
		// path may be absolute; check line:content form
		if !strings.Contains(got, ":3:// TODO here") {
			t.Errorf("missing hit line: %q", got)
		}
	}
	// Context lines use -
	if !strings.Contains(got, "-2-// prev") {
		t.Errorf("missing before context: %q", got)
	}
	if !strings.Contains(got, "-4-// next") {
		t.Errorf("missing after context: %q", got)
	}
}

func TestCLISmokeContextNegative(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", ".", "-C", "-1"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for negative context")
	}
	if !strings.Contains(err.Error(), "context must be >= 0") {
		t.Errorf("error = %v, want context validation message", err)
	}
}

func TestCLISmokeWorkersNegative(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", ".", "--workers", "-1"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for negative workers")
	}
	if !strings.Contains(err.Error(), "workers must be >= 0") {
		t.Errorf("error = %v, want workers validation message", err)
	}
}

func TestCLISmokeWorkersOne(t *testing.T) {
	// --workers 1 must still find hits (wiring reaches searcher.Options.Workers).
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a\n// TODO here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go", "--workers", "1", "--no-color"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nout=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "TODO here") {
		t.Errorf("--workers 1 output missing hit: %q", out.String())
	}
}

func TestCLISmokeNoGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.skip\n"), 0o644); err != nil {
		t.Fatalf("WriteFile gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.go"), []byte("package keep\n// TODO keep\n"), 0o644); err != nil {
		t.Fatalf("WriteFile keep: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "noise.skip"), []byte("package noise\n// TODO noise\n"), 0o644); err != nil {
		t.Fatalf("WriteFile noise: %v", err)
	}

	// Default: .gitignore hides noise.skip
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go,skip", "--no-color"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute default: %v\nout=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "TODO keep") {
		t.Errorf("default missing keep hit: %q", out.String())
	}
	if strings.Contains(out.String(), "TODO noise") {
		t.Errorf("default should hide noise.skip: %q", out.String())
	}

	// --no-gitignore: both hits
	out.Reset()
	cmd = newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go,skip", "--no-gitignore", "--no-color"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute --no-gitignore: %v\nout=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "TODO keep") || !strings.Contains(out.String(), "TODO noise") {
		t.Errorf("--no-gitignore want both hits: %q", out.String())
	}
}

func TestCLISmokeNoColor(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, []byte("package a\n// TODO here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go", "--no-color"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nout=%q", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "TODO here") {
		t.Errorf("missing hit: %q", got)
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("unexpected ANSI codes with --no-color: %q", got)
	}
}

func TestCLISmokeIgnore(t *testing.T) {
	root := t.TempDir()
	// Keyword only under ignored basename path.
	if err := os.WriteFile(filepath.Join(root, "keep.go"), []byte("package keep\n// no hit\n"), 0o644); err != nil {
		t.Fatalf("WriteFile keep: %v", err)
	}
	secretDir := filepath.Join(root, "secret")
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secretDir, "a.go"), []byte("package secret\n// TODO hidden\n"), 0o644); err != nil {
		t.Fatalf("WriteFile secret: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go", "--ignore", "secret"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nout=%q", err, out.String())
	}
	if got := out.String(); got != "" {
		t.Errorf("want empty output when hit is under --ignore, got %q", got)
	}
}

func TestCLISmokeDefaultPath(t *testing.T) {
	// Keyword-only args should search under "." (current working directory).
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n// TODO here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", "--ext", "go", "--no-color"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nout=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "TODO here") {
		t.Errorf("default path output missing hit: %q", out.String())
	}
}

func TestCLISmokeSkipWarning(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.go")
	bad := filepath.Join(root, "bad.go")
	if err := os.WriteFile(good, []byte("package good\n// TODO ok\n"), 0o644); err != nil {
		t.Fatalf("WriteFile good: %v", err)
	}
	if err := os.WriteFile(bad, []byte("package bad\n// TODO no\n"), 0o644); err != nil {
		t.Fatalf("WriteFile bad: %v", err)
	}
	if err := os.Chmod(bad, 0); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o644) })

	if f, err := os.Open(bad); err == nil {
		f.Close()
		t.Skip("unreadable file still openable (root?); skip warning test")
	}

	var stdout, stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}

	if !strings.Contains(stdout.String(), "TODO ok") {
		t.Errorf("stdout missing good hit: %q", stdout.String())
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "fsearch: skip") {
		t.Errorf("stderr missing skip prefix: %q", errOut)
	}
	if !strings.Contains(errOut, bad) {
		t.Errorf("stderr missing bad path: %q", errOut)
	}
	// Hits must not leak onto stderr; warnings must not leak onto stdout.
	if strings.Contains(stdout.String(), "fsearch: skip") {
		t.Errorf("skip warning leaked to stdout: %q", stdout.String())
	}
}
