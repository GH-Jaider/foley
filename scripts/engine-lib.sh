#!/bin/sh
# Construye libghostty-vt.a PINEADA.
#
# Uso: scripts/engine-lib.sh [goos-goarch ...]
#   Sin argumentos construye los 4 targets de v1; con argumentos, solo los
#   pedidos (p. ej. `scripts/engine-lib.sh linux-amd64` en CI). Un target
#   desconocido es error inmediato: un typo jamás debe ser un no-op verde.
#
# POSIX sh a propósito: corre en macOS (bash 3.2 eterno) y en CI Linux.
# El build corre en Docker (zig 0.15.x no linka en macOS 26); el contenedor
# detecta su propia arquitectura para bajar el zig correcto. Los targets
# darwin se re-empacan con libtool de Apple (alineación del archiver de
# zig) cuando el host es macOS.
#
# Regenera include/ (headers + LICENSE) desde el paquete del pin VERIFICADO:
# `zig fetch` computa el hash con el mismo algoritmo que usa el build de la
# .a y se compara contra build.zig.zon — headers y .a no pueden divergir ni
# entre sí ni del pin. Nunca editar include/ a mano.
set -eu

cd "$(dirname "$0")/.."
root=$(pwd)
pkg=internal/vtengine/ghostty
img=foley-engine-build
zon=$pkg/libbuild/build.zig.zon

for t in "$@"; do
  case $t in
    linux-arm64 | linux-amd64 | darwin-arm64 | darwin-amd64) ;;
    *)
      echo "engine-lib: target desconocido '$t'" >&2
      echo "  validos: linux-arm64 linux-amd64 darwin-arm64 darwin-amd64" >&2
      exit 1
      ;;
  esac
done
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

# retry: el cliente HTTP de zig 0.15 es flaky (HttpConnectionClosing en
# fetches de deps). El volumen foley-zig-cache persiste /root/.cache/zig
# entre corridas para que cada reintento avance en vez de partir de cero.
# `make engine-clean` retira volumen e imagen (p. ej. tras un bump de pin).
retry() {
  n=0
  until "$@"; do
    n=$((n + 1))
    if [ "$n" -ge 3 ]; then
      echo "engine-lib: fallo tras $n intentos" >&2
      return 1
    fi
    echo "engine-lib: fallo transitorio — reintento $n/3" >&2
  done
}

docker build -q -t "$img" - <<'EOF' >/dev/null
FROM debian:bookworm
RUN apt-get update -qq && apt-get install -y -qq curl xz-utils ca-certificates >/dev/null && rm -rf /var/lib/apt/lists/*
RUN cd /opt && arch=$(uname -m) \
    && (curl -fsSLO --retry 3 "https://ziglang.org/download/0.15.2/zig-${arch}-linux-0.15.2.tar.xz" \
        || curl -fsSLO --retry 3 "https://ziglang.org/download/0.15.2/zig-linux-${arch}-0.15.2.tar.xz") \
    && tar xf zig-*.tar.xz && rm zig-*.tar.xz && mv zig-* zig
ENV PATH="/opt/zig:${PATH}"
EOF

# --- headers desde el pin, verificados --------------------------------------
url=$(sed -n 's/.*\.url = "\(.*\)",/\1/p' "$zon")
want=$(sed -n 's/.*\.hash = "\(.*\)",/\1/p' "$zon")
if [ -z "$url" ] || [ -z "$want" ]; then
  echo "engine-lib: no pude leer url/hash del pin en $zon" >&2
  exit 1
fi
echo "engine-lib: verificando el pin con zig fetch"
got=$(retry docker run --rm -v foley-zig-cache:/root/.cache/zig "$img" zig fetch "$url")
if [ "$got" != "$want" ]; then
  echo "engine-lib: HASH MISMATCH del pin" >&2
  echo "  esperado: $want" >&2
  echo "  obtenido: $got" >&2
  exit 1
fi
echo "engine-lib: refrescando include/ desde el paquete verificado"
docker run --rm -v foley-zig-cache:/root/.cache/zig -v "$root":/repo "$img" \
  sh -c "rm -rf /repo/$pkg/include \
    && cp -R /root/.cache/zig/p/$want/include /repo/$pkg/include \
    && cp /root/.cache/zig/p/$want/LICENSE /repo/$pkg/include/LICENSE \
    && chmod -R a+rwX /repo/$pkg/include"

# --- .a por target (ABI gnu explícita: los runners de CI son glibc) ---------
build() {
  zt=$1
  out=$2
  wants "$out" || return 0
  echo "engine-lib: $zt -> lib/$out"
  retry docker run --rm -v foley-zig-cache:/root/.cache/zig \
    -v "$root":/repo -w "/repo/$pkg/libbuild" "$img" \
    zig build "-Dtarget=$zt" --prefix "zig-out/$out"
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
