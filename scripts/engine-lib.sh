#!/bin/sh
# Construye libghostty-vt.a PINEADA (ADR-010).
#
# Uso: scripts/engine-lib.sh [goos-goarch ...]
#   Sin argumentos construye los 4 targets de v1; con argumentos, solo los
#   pedidos (p. ej. `scripts/engine-lib.sh linux-amd64` en CI).
#
# POSIX sh a propósito: corre en macOS (bash 3.2 eterno) y en CI Linux.
# El build corre en Docker (zig 0.15.x no linka en macOS 26); el contenedor
# detecta su propia arquitectura para bajar el zig correcto. Los targets
# darwin se re-empacan con libtool de Apple (alineación del archiver de
# zig) cuando el host es macOS.
#
# Regenera include/ (headers + LICENSE) DESDE EL MISMO TARBALL DEL PIN para
# que headers y .a no puedan divergir. Nunca editar include/ a mano.
set -eu

cd "$(dirname "$0")/.."
root=$(pwd)
pkg=internal/vtengine/ghostty
img=foley-engine-build
wanted="$*"

wants() {
  [ -z "$wanted" ] && return 0
  case " $wanted " in
    *" $1 "*) return 0 ;;
    *) return 1 ;;
  esac
}

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
RUN cd /opt && arch=$(uname -m) \
    && (curl -fsSLO "https://ziglang.org/download/0.15.2/zig-${arch}-linux-0.15.2.tar.xz" \
        || curl -fsSLO "https://ziglang.org/download/0.15.2/zig-linux-${arch}-0.15.2.tar.xz") \
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
# El cliente HTTP de zig 0.15 es flaky (HttpConnectionClosing en fetches de
# deps transitivas). El volumen persiste /root/.cache/zig entre corridas para
# que cada reintento avance en vez de re-fetchear todo desde cero.
build() {
  zt=$1
  out=$2
  wants "$out" || return 0
  echo "engine-lib: $zt -> lib/$out"
  n=0
  until docker run --rm -v foley-zig-cache:/root/.cache/zig \
      -v "$root":/repo -w "/repo/$pkg/libbuild" "$img" \
      zig build "-Dtarget=$zt" --prefix "zig-out/$out"; do
    n=$((n + 1))
    if [ "$n" -ge 3 ]; then
      echo "engine-lib: $out falló tras $n intentos" >&2
      exit 1
    fi
    echo "engine-lib: fetch flaky de zig — reintento $n/3"
  done
  mkdir -p "$pkg/lib/$out"
  cp "$pkg/libbuild/zig-out/$out/lib/libghostty-vt-static.a" "$pkg/lib/$out/libghostty-vt.a"
}

build aarch64-linux-gnu linux-arm64
build x86_64-linux-gnu linux-amd64
build aarch64-macos darwin-arm64
build x86_64-macos darwin-amd64

# --- re-empacado darwin (solo posible donde vive libtool de Apple) ----------
if [ "$(uname -s)" = Darwin ]; then
  for out in darwin-arm64 darwin-amd64; do
    wants "$out" || continue
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
  echo "engine-lib: AVISO — targets darwin (si se pidieron) sin re-empacar: correr en macOS"
fi

# --- checksums ---------------------------------------------------------------
for dir in "$pkg"/lib/*/; do
  a=${dir%/}/libghostty-vt.a
  [ -f "$a" ] || continue
  sha256 "$a" > "$a.sha256"
  printf 'engine-lib: %s...  %s\n' "$(cut -c1-16 <"$a.sha256")" "$a"
done

echo "engine-lib: listo"
