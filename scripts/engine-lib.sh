#!/bin/sh
# Construye libghostty-vt.a PINEADA para los targets de v1 (ADR-010).
#
# POSIX sh a propósito: este script corre en macOS (donde /bin/bash es 3.2
# desde 2007 y jamás se actualizará — GPLv3) Y en CI Linux. Nada de
# bashismos; shellcheck -s sh es el gate.
#
# El pin vive en internal/vtengine/ghostty/libbuild/build.zig.zon (URL del
# commit + hash de contenido verificado por zig). El build corre en Docker
# porque zig 0.15.x no linka en macOS 26 (SDK). Los targets darwin salen
# cross-compilados y se re-empacan con libtool de Apple (su ld exige una
# alineación de miembros que el archiver de zig no produce).
#
# Además regenera include/ (headers + LICENSE) DESDE EL MISMO TARBALL DEL
# PIN, para que headers y .a no puedan divergir. Nunca editar include/ a
# mano.
#
# Salida: internal/vtengine/ghostty/lib/<goos-goarch>/libghostty-vt.a (+ .sha256)
# — gitignoradas: reproducibles desde el pin.
set -eu

cd "$(dirname "$0")/.."
root=$(pwd)
pkg=internal/vtengine/ghostty
img=foley-engine-build

sha256() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    sha256sum "$1" | cut -d' ' -f1
  fi
}

docker build -q -t "$img" - <<'EOF' >/dev/null
FROM debian:bookworm
RUN apt-get update -qq && apt-get install -y -qq curl xz-utils ca-certificates >/dev/null && rm -rf /var/lib/apt/lists/*
RUN cd /opt && (curl -fsSLO https://ziglang.org/download/0.15.2/zig-aarch64-linux-0.15.2.tar.xz \
    || curl -fsSLO https://ziglang.org/download/0.15.2/zig-linux-aarch64-0.15.2.tar.xz) \
    && tar xf zig-*.tar.xz && rm zig-*.tar.xz && mv zig-* zig
ENV PATH="/opt/zig:${PATH}"
EOF

# --- headers desde el pin ---------------------------------------------------
url=$(sed -n 's/.*\.url = "\(.*\)",/\1/p' "$pkg/libbuild/build.zig.zon")
[ -n "$url" ] || { echo "engine-lib: no pude leer la URL del pin" >&2; exit 1; }
echo "engine-lib: refrescando include/ desde el pin ($url)"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" | tar -xz -C "$tmp"
srcdir=$(find "$tmp" -maxdepth 1 -mindepth 1 -type d | head -1)
rm -rf "$pkg/include"
cp -R "$srcdir/include" "$pkg/include"
cp "$srcdir/LICENSE" "$pkg/include/LICENSE"

# --- .a por target (ABI gnu explícita: los runners de CI son glibc) ---------
printf '%s\n' \
  "aarch64-linux-gnu linux-arm64" \
  "x86_64-linux-gnu linux-amd64" \
  "aarch64-macos darwin-arm64" \
  "x86_64-macos darwin-amd64" |
while read -r zt out; do
  echo "engine-lib: $zt -> lib/$out"
  docker run --rm -v "$root":/repo -w "/repo/$pkg/libbuild" "$img" \
    zig build "-Dtarget=$zt" --prefix "zig-out/$out"
  mkdir -p "$pkg/lib/$out"
  cp "$pkg/libbuild/zig-out/$out/lib/libghostty-vt-static.a" "$pkg/lib/$out/libghostty-vt.a"
done

# --- re-empacado darwin (solo posible donde vive libtool de Apple) ----------
if [ "$(uname -s)" = Darwin ]; then
  for out in darwin-arm64 darwin-amd64; do
    dir=$pkg/lib/$out
    [ -f "$dir/libghostty-vt.a" ] || continue
    work=$(mktemp -d)
    (
      cd "$work"
      ar x "$root/$dir/libghostty-vt.a"
      chmod 644 ./*
      libtool -static -o "$root/$dir/libghostty-vt.a" ./*.o
    )
    rm -rf "$work"
    echo "engine-lib: $out re-empacada con libtool"
  done
else
  echo "engine-lib: AVISO — targets darwin sin re-empacar (correr en macOS o repack en CI)"
fi

# --- checksums ---------------------------------------------------------------
for dir in "$pkg"/lib/*/; do
  a=$dir/libghostty-vt.a
  [ -f "$a" ] || continue
  sha256 "$a" > "$a.sha256"
  printf 'engine-lib: %s...  %s\n' "$(cut -c1-16 <"$a.sha256")" "$a"
done

echo "engine-lib: listo"
