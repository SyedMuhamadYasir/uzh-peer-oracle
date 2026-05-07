BIN ?= uzh-peer-oracle
PREFIX ?= /usr/local

.PHONY: build test tidy install clean contabo-doctor contabo-oracle contabo-once contabo-loop contabo-before contabo-after contabo-smoke contabo-tmux contabo-bootnodes

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

contabo-doctor:
	./scripts/contabo_doctor.sh

contabo-oracle:
	./scripts/contabo_oracle_up.sh

contabo-once:
	./scripts/contabo_agent_once.sh

contabo-loop:
	./scripts/contabo_agent_loop.sh

contabo-before:
	./scripts/contabo_measure_before.sh

contabo-after:
	./scripts/contabo_measure_after.sh

contabo-smoke:
	./scripts/contabo_smoke.sh

contabo-tmux:
	./scripts/contabo_tmux_up.sh

contabo-bootnodes:
	./scripts/contabo_bootnodes_toml.sh
