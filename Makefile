GOLANGCI_VERSION := 2.12.2

.PHONY: build test test-race lint lint-sh fmt vuln codemap fixtures fonts e2e-linux e2e-darwin lint-version engine-lib engine-clean

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

lint: lint-version lint-sh
	golangci-lint run

# shellcheck lee el shebang de cada script y aplica el dialecto correcto
# (engine-lib.sh es POSIX sh; codemap-check.sh es bash 3.2-compatible).
# Los scripts de examples/ se shippean: mismo gate.
lint-sh:
	shellcheck scripts/*.sh examples/*/*.sh

fmt: lint-version
	golangci-lint fmt

lint-version:
	@golangci-lint version 2>/dev/null | grep -qF "$(GOLANGCI_VERSION)" || { \
		echo "se requiere golangci-lint v$(GOLANGCI_VERSION) — instala con:"; \
		echo "  curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b \$$(go env GOPATH)/bin v$(GOLANGCI_VERSION)"; \
		exit 1; }

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

codemap:
	scripts/codemap-check.sh

fixtures:
	go build -o internal/vtengine/ghostty/testdata/bin/keyprobe ./internal/vtengine/ghostty/testdata/keyprobe

e2e-linux e2e-darwin:
	@echo "$@: llega en milestones posteriores (ver docs/ROADMAP.md)"; exit 1

engine-lib:
	scripts/engine-lib.sh

# Retira los artefactos Docker de engine-lib (volumen-cache de zig + imagen).
# El cache acelera rebuilds del mismo pin; tras un bump de pin o para
# recuperar disco, esto es todo lo que hay que borrar.
engine-clean:
	docker volume rm -f foley-zig-cache
	docker rmi -f foley-engine-build

fonts:
	scripts/fonts.sh
