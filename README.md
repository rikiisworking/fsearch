# fsearch

Fast recursive file content search for the Linux shell.

Modern, concurrent alternative to classic `grep` / `find` combos.

> **Status:** Sprint 0 foundation. Search engine lands in Sprint 1.

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

# Coming soon (Sprint 1+):
# ./bin/fsearch "TODO" . --ext go,md
```

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
