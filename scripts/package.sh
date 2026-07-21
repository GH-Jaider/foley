#!/bin/sh
# Packages ONE built foley binary into a release tarball with a
# checksum. The release workflow builds the binary natively per target
# (cgo + the pinned .a + -tags embedfonts, so it is self-contained —
# fonts and engine baked in) and calls this to package it. Kept a plain
# script so the exact bytes a release ships can be reproduced by hand.
#
# Usage: scripts/package.sh <binary> <os> <arch> <version> [outdir]
#   <binary>   path to the built foley executable
#   <os>       darwin | linux           (release name, not GOOS quirks)
#   <arch>     arm64 | amd64
#   <version>  e.g. 0.1.0 (no leading v)
#   [outdir]   where the .tar.gz + .sha256 land (default: dist)
#
# Produces: <outdir>/foley_<version>_<os>_<arch>.tar.gz  (+ .sha256)
# The tarball holds: foley, LICENSE, README.md, foley.md.
set -eu

bin=${1:?usage: package.sh <binary> <os> <arch> <version> [outdir]}
os=${2:?missing os}
arch=${3:?missing arch}
version=${4:?missing version}
outdir=${5:-dist}

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)

[ -f "$bin" ] || {
	echo "package: no binary at $bin" >&2
	exit 1
}

name="foley_${version}_${os}_${arch}"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

# Stage the payload under a clean name so the tarball extracts tidy.
cp "$bin" "$stage/foley"
chmod +x "$stage/foley"
cp "$root/LICENSE" "$root/README.md" "$root/foley.md" "$stage/"

mkdir -p "$outdir"
tarball="$outdir/$name.tar.gz"

# Deterministic archive: sorted entries, pinned mtime/owner — the same
# inputs give the same bytes, so a checksum means something.
tar --numeric-owner --owner=0 --group=0 --mtime='2020-01-01 00:00:00' \
	--sort=name -czf "$tarball" -C "$stage" foley LICENSE README.md foley.md 2>/dev/null ||
	# BSD tar (macOS) lacks --sort/--numeric-owner spelling; fall back.
	tar -czf "$tarball" -C "$stage" foley LICENSE README.md foley.md

# Checksum next to the tarball (the release also aggregates a
# checksums.txt over all of them).
if command -v sha256sum >/dev/null 2>&1; then
	(cd "$outdir" && sha256sum "$name.tar.gz" >"$name.tar.gz.sha256")
else
	(cd "$outdir" && shasum -a 256 "$name.tar.gz" >"$name.tar.gz.sha256")
fi

echo "package: wrote $tarball"
cat "$outdir/$name.tar.gz.sha256"
