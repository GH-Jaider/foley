# AGENTS.md — working contract for foley (AI agents and humans)

foley is a Go library + CLI that **renders** scripted terminal demos — no window, no screen capture: the app runs on a real pty, an embedded terminal engine (libghostty-vt) keeps the screen state, and foley rasterizes every frame itself. Output is byte-identical on macOS, Linux and CI, with zero permissions.

## Commands

| Task | Command |
|---|---|
| Build / test (works out of the box, fake engine) | `make build`, `make test` |
| Format / lint — the gate | `make fmt`, `make lint` |
| Real-engine suite | `make engine-lib fonts fixtures`, then `go test -tags ghosttyvt ./...` |
| Vulnerability check | `make vuln` |

Toolchain pins: Go per `go.mod`, golangci-lint per `GOLANGCI_VERSION` in the `Makefile`. `make codemap` is a maintainer-only check that no-ops on external clones.

## Boundaries (enforced by depguard, not good faith)

- **All cgo lives in `internal/vtengine/ghostty`** (pinned libghostty-vt). Only the factory (`internal/vtengine/factory`) knows concrete engines — driver, raster and the public API depend on the contract package `internal/vtengine`.
- **External binaries** (ffmpeg, gifski) are invoked only via `internal/execx`. No bare `exec.Command` in any other package.
- **Nobody parses VT outside the engine**; nobody touches font files outside `fontpack`/`raster`.
- `cmd/foley` only consumes the public API (root package `foley`) — no business logic in the CLI.
- `tape/internal/vhsgrammar` is vendored from VHS: **never edit it by hand**. Stringly-typed data must not cross out of `tape/`.

## Strong typing

- Nothing stringly outside the vendored-grammar quarantine: units, bounded domains and invariants get their own type (`key.Key`, `time.Duration`, enums with exhaustive `switch` — the `exhaustive` linter verifies it).
- Zero `any` / `map[string]any` in the public API.

## Dependency budget

stdlib + `creack/pty` + `golang.org/x/sys` (+ `x/term`) + the text stack (`go-text/typesetting`, `golang.org/x/image`); cgo only in the quarantine. The executable list is depguard's `allow` in `.golangci.yml`. Any other dependency requires a justified discussion in an issue first.

## Invariants

- No global state; no `init()` with effects.
- `context.Context` first in every blocking operation.
- Errors wrapped with `%w`; exported sentinels.
- **Shell scripts are POSIX `sh`** (`#!/bin/sh`) — macOS will never ship modern bash (`/bin/bash` is 3.2) and scripts live both on Mac and CI-Linux. `shellcheck` (via `make lint`) is the gate. If a script grows real logic, promote it to Go tooling.
- **Performance is a requirement:** startup < 1 s; deterministic mode renders faster than real time; the raster hot path has a budget (< 2 ms/partial frame 120×30@2x).

## Testing by layer

- driver / raster → against the fake engine (`internal/vtengine/fake`).
- engines → against the `enginetest` conformance suite; an engine is "done" when the unmodified suite passes.
- raster → byte-exact typographic golden suite (visual regressions are impossible by construction).
- tape parser → vendored VHS corpus + fuzz.

`make fmt lint test` green before anything is called done. Formatting and lint are not negotiable: a `golangci-lint fmt` diff is a red build.

## Pull requests

- One change = one coherent commit/PR. Keep diffs minimal and follow the existing style.
- Architecture or dependency changes: open an issue first.
