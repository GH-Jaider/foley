#!/bin/sh
# Descarga las fuentes PINEADAS del fontpack (versión exacta + sha256).
# Los hashes canónicos viven en internal/fontpack/fontpack.go (el paquete
# los verifica en cada Load); este script debe coincidir con ellos.
# POSIX sh: corre igual en macOS y CI Linux. Salida gitignorada.
set -eu

cd "$(dirname "$0")/.."
dir=internal/fontpack/fonts
mkdir -p "$dir"

sha256() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    sha256sum "$1" | cut -d' ' -f1
  fi
}

fetch() {
  name=$1
  url=$2
  want=$3
  dst=$dir/$name
  if [ -f "$dst" ] && [ "$(sha256 "$dst")" = "$want" ]; then
    echo "fonts: $name ok (cache)"
    return 0
  fi
  echo "fonts: bajando $name"
  curl -fsSL -o "$dst" "$url"
  got=$(sha256 "$dst")
  if [ "$got" != "$want" ]; then
    echo "fonts: HASH MISMATCH en $name" >&2
    echo "  esperado: $want" >&2
    echo "  obtenido: $got" >&2
    rm -f "$dst"
    exit 1
  fi
  echo "fonts: $name ok"
}

jb=https://raw.githubusercontent.com/JetBrains/JetBrainsMono/v2.304/fonts/ttf
noto=https://raw.githubusercontent.com/googlefonts/noto-emoji/v2.047/fonts

fetch JetBrainsMono-Regular.ttf "$jb/JetBrainsMono-Regular.ttf" \
  a0bf60ef0f83c5ed4d7a75d45838548b1f6873372dfac88f71804491898d138f
fetch JetBrainsMono-Bold.ttf "$jb/JetBrainsMono-Bold.ttf" \
  5590990c82e097397517f275f430af4546e1c45cff408bde4255dad142479dcb
fetch JetBrainsMono-Italic.ttf "$jb/JetBrainsMono-Italic.ttf" \
  9d0a1f7a708e6af183f1193b7e81d40da294f5c67682c085d8401c60aac8ded4
fetch NotoColorEmoji.ttf "$noto/NotoColorEmoji.ttf" \
  39ee3c587e10e89669b9ff32703261d10d5f9c4dd5ad147b6b5a1c5200591817

echo "fonts: listo"
