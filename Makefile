GOLANGCI_VERSION := 2.12.2

.PHONY: build test test-race lint lint-sh fmt vuln codemap fixtures fonts examples e2e-linux e2e-darwin lint-version engine-lib engine-clean terminfo

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

lint: lint-version lint-sh lint-completions
	golangci-lint run

# shellcheck lee el shebang de cada script y aplica el dialecto correcto
# (engine-lib.sh es POSIX sh; codemap-check.sh es bash 3.2-compatible).
# Los scripts de examples/ se shippean: mismo gate.
lint-sh:
	shellcheck scripts/*.sh examples/*/*.sh

# Los scripts de `foley completion` viven como consts en Go (fuera del
# alcance de shellcheck): cada shell presente valida el suyo con su
# parser real (-n); un shell ausente se salta CON AVISO, jamás se finge.
lint-completions:
	@for sh in bash zsh fish; do \
		if command -v $$sh >/dev/null 2>&1; then \
			go run ./cmd/foley completion $$sh | $$sh -n || exit 1; \
			echo "completion $$sh: syntax ok"; \
		else \
			echo "completion $$sh: shell no instalado — check saltado"; \
		fi; \
	done

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

# Regenera la entrada terminfo pineada (internal/terminfo) desde el pin
# del motor — correr tras un bump de build.zig.zon; el diff va en el
# mismo commit del bump. Herramientas de mantenedor: zig + tic.
terminfo:
	scripts/terminfo.sh

# Re-record every example gif from its tape (they ARE the docs the
# README embeds). `make examples` for all; narrow with the script:
# scripts/examples.sh fetch keys
examples:
	scripts/examples.sh

# Regenerate the brand assets from the tape: the logo IS a recording
# (assets/logo/logo.tape -> real engine frames -> film strip).
logo:
	cd assets/logo && FOLEY_FONTS=$(CURDIR)/internal/fontpack/fonts go run -tags ghosttyvt ../../cmd/foley -cols 8 -rows 3 logo.tape
	go run ./tooling/logogen assets/logo assets/logo
