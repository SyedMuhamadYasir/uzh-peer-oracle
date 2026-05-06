BIN ?= uzh-peer-oracle
PREFIX ?= /usr/local

.PHONY: build test tidy install clean

build:
	go build -trimpath -ldflags="-s -w" -o bin/$(BIN) ./cmd/uzh-peer-oracle

test:
	go test ./...

tidy:
	go mod tidy

install: build
	install -m 0755 bin/$(BIN) $(PREFIX)/bin/$(BIN)

clean:
	rm -rf bin
