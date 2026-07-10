# fsearch

Fast recursive file content search for the Linux shell.

Modern, concurrent alternative to classic `grep` / `find` combos.

> **Status:** Sprint 1 — core search works. Color/pretty output lands in Sprint 2.

## Requirements

- Go 1.22+ (tested with Go 1.26)
- Linux

## Build

```bash
make build
# or
go build -o bin/fsearch ./cmd/fsearch
```

## Install

```bash
make install
# or
go install ./cmd/fsearch
```

## Usage

```bash
fsearch --help

# Search for a keyword under the current directory
./bin/fsearch "TODO" .

# Only Go and Markdown files
./bin/fsearch "TODO" . --ext go,md

# Extra basename ignores (repeatable)
./bin/fsearch "FIXME" ./internal --ignore vendor --ignore '*.min.js'
```

Output is grep-style: `path:line:content`

| Flag | Meaning |
|------|---------|
| `--ext go,md` | only these extensions (empty = all) |
| `--ignore PAT` | skip basenames matching PAT (exact or glob; repeatable) |

## Develop

```bash
make test
make cover
make clean
```

## Project layout

```
cmd/fsearch/     CLI entry (cobra)
internal/
  searcher/      content matching
  walker/        directory walk
  output/        result formatting
  ignore/        skip patterns
```

## Docs

- [AGENTS.md](AGENTS.md) — agent/dev rules
- [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md) — sprint plan
