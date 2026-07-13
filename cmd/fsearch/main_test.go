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
		name     string
		keyword  string
		root     string
		exts     string
		ignores  []string
		wantOpts searcher.Options
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildOptions(tt.keyword, tt.root, tt.exts, tt.ignores)
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
