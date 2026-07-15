.PHONY: build install test cover bench clean run man

BINARY := bin/fsearch
PKG    := ./cmd/fsearch
MAN    := docs/fsearch.1

build:
	go build -o $(BINARY) $(PKG)

# Installs to ~/.local/bin and ensures that dir is on PATH (see scripts/install.sh).
install:
	PKG=$(PKG) ./scripts/install.sh

test:
	go test ./... -v

cover:
	go test ./... -cover

# Searcher benchmarks (walk + concurrent scan). Override with:
#   make bench BENCH=BenchmarkSearch BENCHTIME=2s
BENCH ?= .
BENCHTIME ?= 1s
bench:
	go test ./internal/searcher -run='^$$' -bench=$(BENCH) -benchmem -benchtime=$(BENCHTIME)

# View the local man page (does not install into the system manpath).
man:
	man ./$(MAN)

clean:
	rm -rf bin/

run: build
	./$(BINARY) --help
