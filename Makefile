.PHONY: build install test cover clean run

BINARY := bin/fsearch
PKG    := ./cmd/fsearch

build:
	go build -o $(BINARY) $(PKG)

install:
	go install $(PKG)

test:
	go test ./... -v

cover:
	go test ./... -cover

clean:
	rm -rf bin/

run: build
	./$(BINARY) --help
