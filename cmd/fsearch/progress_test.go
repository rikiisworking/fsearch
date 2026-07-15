package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestShouldShowProgress(t *testing.T) {
	// Non-TTY buffer never shows progress.
	var buf bytes.Buffer
	if shouldShowProgress(&buf, false, false) {
		t.Error("buffer should not show progress")
	}
	if shouldShowProgress(os.Stderr, true, false) {
		t.Error("jsonOut should disable progress")
	}
	if shouldShowProgress(os.Stderr, false, true) {
		t.Error("noProgress should disable progress")
	}
	// Real stderr may or may not be a TTY in CI; only check the false paths above.
}

func TestProgressWriterFileDoneAndFinish(t *testing.T) {
	var buf bytes.Buffer
	p := newProgressWriter(&buf)
	p.minGap = 0 // no rate limit in tests

	p.fileDone("a.go", 2)
	p.fileDone("b.go", 0)
	p.finish()

	got := buf.String()
	if !strings.Contains(got, "fsearch: 2 files, 2 matches") {
		t.Errorf("got %q, want final counts 2 files, 2 matches", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("finish should end with newline: %q", got)
	}
}

func TestProgressWriterRateLimit(t *testing.T) {
	var buf bytes.Buffer
	p := newProgressWriter(&buf)
	p.minGap = time.Hour // only first render before finish

	p.fileDone("a", 1)
	p.fileDone("b", 1) // suppressed by rate limit
	// finish forces write with latest counts
	p.finish()

	// Intermediate may have one \r update; final line has both files.
	if !strings.Contains(buf.String(), "2 files, 2 matches") {
		t.Errorf("finish should show final totals: %q", buf.String())
	}
}

func TestProgressWriterPadsShorterLine(t *testing.T) {
	var buf bytes.Buffer
	p := newProgressWriter(&buf)
	p.minGap = 0

	// Establish a wide line, then a shorter one (force width via lineWidth).
	p.fileDone("a", 1)
	p.mu.Lock()
	wide := p.lineWidth
	p.lineWidth = wide + 10 // pretend a longer message was shown
	p.mu.Unlock()

	p.finish()
	got := buf.String()
	// Final line should be padded to the inflated width before newline.
	if idx := strings.LastIndex(got, "\r"); idx >= 0 {
		line := strings.TrimSuffix(got[idx+1:], "\n")
		if len(line) < wide+10 {
			t.Errorf("padded line len = %d, want >= %d: %q", len(line), wide+10, line)
		}
	} else {
		t.Errorf("expected \\r in output: %q", got)
	}
}

func TestCLISmokeJSONNoProgressOnBuffer(t *testing.T) {
	// With SetErr to a buffer (non-TTY), progress must stay silent even without --no-progress.
	// Also --json disables progress regardless of TTY.
	root := t.TempDir()
	path := root + "/a.go"
	if err := os.WriteFile(path, []byte("package a\n// TODO\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go", "--json"})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if stderr.String() != "" {
		t.Errorf("stderr should be empty with --json + buffer: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "TODO") {
		t.Errorf("stdout missing hit: %q", stdout.String())
	}
}

func TestCLISmokeNoProgressFlag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/a.go", []byte("package a\n// TODO\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"TODO", root, "--ext", "go", "--no-progress", "--no-color"})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if stderr.String() != "" {
		t.Errorf("stderr should be empty with --no-progress: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "TODO") {
		t.Errorf("stdout missing hit: %q", stdout.String())
	}
}
