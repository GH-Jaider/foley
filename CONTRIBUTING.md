# Contributing to foley

Thanks for stopping by! Bugs, fixes and ideas are welcome.

- **Small fixes** (typos, bugs with a clear repro): go straight to a PR.
- **Features, architecture or dependency changes**: open an issue first — the project keeps a tight dependency budget and hard package boundaries (see `AGENTS.md`).

## Setup

- Go (version in `go.mod`) and golangci-lint (version in the `Makefile`'s `GOLANGCI_VERSION`).
- `go build ./...` and `go test ./...` work out of the box, against the fake terminal engine.
- The real-engine suite needs the pinned library and fonts: `make engine-lib fonts fixtures`, then `go test -tags ghosttyvt ./...` (`engine-lib` uses Docker).

## The gate

`make fmt lint test` must be green — formatting and lint are not negotiable, and CI re-runs them. Shell scripts are POSIX `sh` and must pass `shellcheck`.

## House rules

- One change per PR; keep diffs minimal and follow the existing style.
- Never edit `tape/internal/vhsgrammar` — it is vendored from VHS.
- Respect the boundaries in `AGENTS.md`: cgo stays in its quarantine, external binaries go through `internal/execx`, and the CLI stays a thin client of the public API.

Be kind.
