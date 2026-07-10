// Package walker walks a directory tree and yields file paths for searching.
package walker

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Filter decides which directories to prune and which files to include.
// Implemented by ignore.Matcher (and anything else with the same shape).
type Filter interface {
	// SkipDir reports whether a directory basename should be pruned
	// (walk does not enter it).
	SkipDir(name string) bool

	// IncludeFile reports whether a file path should be emitted for search.
	IncludeFile(path string) bool
}

// Walk walks root and sends regular file paths to files.
// Symlinks are skipped. The caller owns files (Walk does not close it).
// Walk stops early if ctx is cancelled.
func Walk(ctx context.Context, root string, filter Filter, files chan<- string) error {
	if filter == nil {
		return fmt.Errorf("walker: filter is nil")
	}

	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("walker: stat root: %w", err)
	}

	// Single file root: emit if the filter allows it.
	if !info.IsDir() {
		if !filter.IncludeFile(root) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case files <- root:
			return nil
		}
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Unreadable entry: skip and keep going.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip symlinks to avoid cycles / surprise targets.
		if d.Type()&fs.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			// Never prune the walk root by basename defaults (e.g. root named "bin").
			if path != root && filter.SkipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}
		if !filter.IncludeFile(path) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case files <- path:
			return nil
		}
	})
	if err != nil {
		return fmt.Errorf("walker: walk: %w", err)
	}
	return nil
}
