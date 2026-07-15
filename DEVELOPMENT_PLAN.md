# fsearch - Development Plan (Grok Build Focus)

**Project**: `fsearch` — Fast recursive file content searcher  
**Language**: Go (Linux shell command)  
**Development Approach**: **Grok Build** (xAI’s coding agent CLI)  
**Primary Goals**: Experience real AI-driven development while practicing:
- Prompt Engineering
- Context Engineering (AGENTS.md + living docs)
- Agile development (short sprints)
- Software Testing

## Project Vision

A fast, user-friendly CLI tool that searches for keywords inside files recursively (including subdirectories). Better UX than plain `grep`/`find` + modern concurrency and options.

### MVP Features
- Recursive keyword search in file contents
- Line numbers + optional context lines
- File extension filtering (`--ext go,md,py`)
- Directory ignore patterns
- Colorful terminal output
- High performance via Go concurrency

### Stretch Goals
- Respect `.gitignore`
- Regex / fuzzy search
- JSON output
- Progress bar
- Interactive result browser

## Grok Build Philosophy for this Project

1. **Always start complex work in Plan Mode** (`/plan ...`)
2. Keep a high-quality `AGENTS.md` (context engineering)
3. Review every plan and every diff
4. Short, focused sprints
5. Write tests alongside (or before) code
6. Update context files after every successful sprint

## Sprint Plan

### Sprint 0: Foundation (Start here)
- Go module + clean project structure
- Cobra CLI skeleton
- AGENTS.md + DEVELOPMENT_PLAN.md + Makefile + README
- Basic `fsearch --help` works

### Sprint 1: Core Search Engine
- Concurrent file walker
- Content matching with line numbers
- Basic ignore + extension filtering
- Unit tests

### Sprint 2: CLI Experience & Output ✅
- Full flags (`--ext`, `--ignore`, `-i`/`--ignore-case`, `-C`/`--context`, `--no-color`)
- Colored output (fatih/color): path, line number, keyword highlight
- Pretty formatting of results (grep-style context + separators)

### Sprint 3: Performance & Robustness ✅
- `.gitignore` support (root file, MVP rule subset, `--no-gitignore`)
- Worker pool / better concurrency control (`--workers`)
- Benchmarks (`make bench`)
- Error handling polish (walk + file skip warnings on stderr)

### Sprint 4: Polish & Extra Features ✅
- JSON output (`--json` NDJSON)
- Regex support (`-e`/`--regex`, Go RE2)
- Progress indicator (stderr TTY; `--no-progress`)
- Installation instructions for Linux

### Sprint 5: Documentation & Release 🚧 in progress
- Excellent README with examples
- Man page or advanced help
- Final code review + cleanup

## Recommended Project Structure

```
fsearch/
├── cmd/
│   └── fsearch/
│       └── main.go
├── internal/
│   ├── searcher/
│   ├── walker/
│   ├── output/
│   └── ignore/
├── docs/
│   └── fsearch.1             # Man page
├── scripts/
│   └── install.sh            # make install helper
├── AGENTS.md                 # ← Critical for Grok Build
├── DEVELOPMENT_PLAN.md
├── Makefile
├── go.mod
├── go.sum
├── README.md
└── .gitignore
```

## Success Criteria

- Tool is fast and pleasant to use daily
- High code quality + good test coverage
- You improved significantly at writing good prompts and maintaining context
- You completed the full Grok Build loop many times (plan → review → approve → test)
