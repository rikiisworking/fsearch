package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// progressWriter prints a single updating status line on stderr.
// Safe for concurrent OnFileDone callbacks.
type progressWriter struct {
	w io.Writer

	files   atomic.Int64
	matches atomic.Int64

	mu        sync.Mutex
	lastShow  time.Time
	minGap    time.Duration
	lineWidth int // max message width seen; used to pad shorter updates
}

func newProgressWriter(w io.Writer) *progressWriter {
	return &progressWriter{
		w:      w,
		minGap: 50 * time.Millisecond,
	}
}

// fileDone records one finished file and maybe refreshes the line.
func (p *progressWriter) fileDone(_ string, matchCount int) {
	if p == nil {
		return
	}
	p.files.Add(1)
	if matchCount > 0 {
		p.matches.Add(int64(matchCount))
	}
	p.render(false)
}

// finish writes a final status line ending with newline.
func (p *progressWriter) finish() {
	if p == nil {
		return
	}
	p.render(true)
}

func (p *progressWriter) render(force bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if !force && !p.lastShow.IsZero() && now.Sub(p.lastShow) < p.minGap {
		return
	}
	p.lastShow = now
	files := p.files.Load()
	matches := p.matches.Load()
	// \r + pad so shorter lines clear leftover chars; final force uses \n.
	msg := fmt.Sprintf("fsearch: %d files, %d matches", files, matches)
	if len(msg) > p.lineWidth {
		p.lineWidth = len(msg)
	} else if len(msg) < p.lineWidth {
		msg += strings.Repeat(" ", p.lineWidth-len(msg))
	}
	if force {
		fmt.Fprintf(p.w, "\r%s\n", msg)
		return
	}
	fmt.Fprintf(p.w, "\r%s", msg)
}

// writerIsTTY reports whether w is an interactive terminal file.
func writerIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// shouldShowProgress is true when progress is allowed for this run.
func shouldShowProgress(stderr io.Writer, jsonOut, noProgress bool) bool {
	if noProgress || jsonOut {
		return false
	}
	return writerIsTTY(stderr)
}
