GOLANGCI_VERSION := 2.12.2

.PHONY: build test test-race lint fmt vuln codemap fixtures e2e-linux e2e-darwin lint-version

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

lint: lint-version
	golangci-lint run

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

fixtures e2e-linux e2e-darwin:
	@echo "$@: llega en milestones posteriores (ver docs/ROADMAP.md)"; exit 1
