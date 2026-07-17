# foley product image (M9): record VHS tapes with zero terminal, zero X,
# zero Chromium. The measuring stick is VHS's own image — 908.2 MB
# compressed (ghcr.io/charmbracelet/vhs amd64, digest 16b21a3bf7bd,
# measured 2026-07-17); the roadmap target is ≤15% of that (~136 MB) and
# this image lands around ~6% (see the oci CI job for the enforced gate).
#
# Build expects the repo prepared like CI does: `make fonts` fetched the
# pinned fonts and `scripts/engine-lib.sh linux-amd64` produced the
# engine .a (both cached in CI). Build:
#
#   docker build -t foley .
#   docker run --rm -v "$PWD":/work foley demo.tape
#
# The binary is FULLY STATIC (glibc static link is safe here: foley uses
# no NSS/DNS — only syscalls, ptys and files), so the runtime stage can
# be alpine/musl. bash ships in the image because every migrated VHS
# tape assumes `Set Shell "bash"` — its absence would break the default
# tape on arrival. ffmpeg comes from the alpine 3.22 branch (base pinned
# by digest; package versions float within the branch, documented
# trade-off until the release pipeline pins a digest-frozen base).

FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -tags ghosttyvt \
    -ldflags '-linkmode external -extldflags "-static"' \
    -o /out/foley ./cmd/foley

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce
RUN apk add --no-cache ffmpeg bash
COPY --from=build /out/foley /usr/local/bin/foley
COPY internal/fontpack/fonts /usr/share/foley/fonts
ENV FOLEY_FONTS=/usr/share/foley/fonts
WORKDIR /work
ENTRYPOINT ["foley"]
