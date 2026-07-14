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
	// SkipDir reports whether a directory should be pruned (walk does not enter it).
	// path is the full directory path; name is filepath.Base(path).
	// path is provided so filters can apply path-relative rules (e.g. .gitignore).
	SkipDir(path, name string) bool

	// IncludeFile reports whether a file path should be emitted for search.
	IncludeFile(path string) bool
}

// Walk walks root and sends regular file paths to files.
// Symlinks are skipped. The caller owns files (Walk does not close it).
// Walk stops early if ctx is cancelled.
//
// onError is optional. When WalkDir reports an entry error (e.g. permission
// denied), the walk continues but onError is called with that path and error
// if non-nil. Cancel errors are returned from Walk, not reported via onError.
func Walk(ctx context.Context, root string, filter Filter, files chan<- string, onError func(path string, err error)) error {
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
			return ctx.Err()
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
			// Unreadable entry: report (optional) and keep going.
			if onError != nil {
				onError(path, walkErr)
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Honor cancellation on every entry (cheap vs readdir/stat).
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
			if path != root && filter.SkipDir(path, d.Name()) {
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
