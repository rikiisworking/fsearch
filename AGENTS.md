# AGENTS.md – fsearch

## Project Overview
fsearch is a fast, concurrent Go CLI tool for recursive keyword search in file contents under a given path (including child directories).  
It aims to be a modern, user-friendly alternative to classic `grep`/`find` combinations.  
Target platform: Linux shell.

## Tech Stack & Preferences
- Language: Go (latest stable)
- CLI framework: cobra
- Concurrency: goroutines + errgroup / semaphore / worker pool
- File walking: filepath.WalkDir
- Colors: github.com/fatih/color
- Testing: table-driven tests with testing package + prefer testify if needed
- No external heavy dependencies for MVP

## Key Commands
- Build: `go build -o bin/fsearch ./cmd/fsearch`
- Install: `go install ./cmd/fsearch`
- Test: `go test ./... -v`
- Test coverage: `go test ./... -cover`
- Lint (if available): `golangci-lint run`
- Run example: `./bin/fsearch "TODO" . --ext go,md -C 1 -i`

## Architecture Rules
- Package layout: cmd/ for entrypoints, internal/ for private packages
- Prefer small, focused packages: searcher, walker, output, ignore
- No global mutable state
- Always pass context.Context where I/O or cancellation makes sense
- Good error wrapping with fmt.Errorf("%w", err)
- Public functions should have clear godoc comments

## Development Rules (Grok Build)
- For any multi-file or non-trivial change → use Plan Mode (`/plan ...`)
- Always write or update unit tests for new logic
- Prefer table-driven tests
- Keep the MVP small and working end-to-end
- After each sprint, update the "Current Focus" section below
- Prefer idiomatic, readable Go over clever code

## Code Style
- gofmt / goimports
- Short functions
- Clear variable names
- Avoid deep nesting
- Early returns

## Current Focus (update this regularly)
**Sprint 0 – Foundation** ✅ complete  
Skeleton, cobra CLI, Makefile, `go build` + `fsearch --help` work.

**Sprint 1 – Core Search Engine** ✅ complete  
Concurrent walker, content matching with line numbers, basic ignore + extension filtering, unit tests.  
End-to-end: `./bin/fsearch "TODO" . --ext go,md`

**Sprint 2 – CLI Experience & Output** ✅ complete  
Flags: `-i`/`--ignore-case`, `-C`/`--context`, `--no-color` (plus `--ext`, `--ignore`).  
Context lines filled on `Match.Before`/`After`; grep-style context print; colored path/line + keyword highlight via fatih/color.  
End-to-end: `./bin/fsearch "TODO" . --ext go,md -C 1 -i`

**Next: Sprint 3 – Performance & Robustness**  
`.gitignore` support, concurrency tuning/benchmarks, error handling polish.

## Future Notes
- Performance is important (should feel snappy on large codebases)
- Output should be beautiful and scannable in the terminal
- Support common developer workflows (ignore node_modules, .git, etc.)

