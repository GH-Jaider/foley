#!/bin/sh
# Vendorea la gramática .tape REAL de VHS (ADR-008), pineada por release.
#
# Trae token/, lexer/, parser/ (código + tests), el corpus de ejemplos
# (*.tape solamente) y la LICENSE, reescribiendo los import paths al
# árbol de foley. Único cambio permitido: esa reescritura. Nunca editar
# tape/internal/vhsgrammar/ a mano — se regenera desde el pin.
set -eu

TAG=v0.11.0
COMMIT=c6af91a25fed05852338a5ed58d9b099b8369a1e
SHA256=c08b8502989fe7e9626c02938f3fc512c2a4ba21f839f455d20d7eb1da7bc39f

cd "$(dirname "$0")/.."
root=$(pwd)
dst=tape/internal/vhsgrammar

sha256() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    sha256sum "$1" | cut -d' ' -f1
  fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "vendor-vhs: bajando vhs $TAG"
curl -fsSL --retry 3 -o "$tmp/vhs.tar.gz" \
  "https://github.com/charmbracelet/vhs/archive/refs/tags/$TAG.tar.gz"
got=$(sha256 "$tmp/vhs.tar.gz")
if [ "$got" != "$SHA256" ]; then
  echo "vendor-vhs: HASH MISMATCH del tarball" >&2
  echo "  esperado: $SHA256" >&2
  echo "  obtenido: $got" >&2
  exit 1
fi
tar -xzf "$tmp/vhs.tar.gz" -C "$tmp"
src=$tmp/vhs-${TAG#v}

rm -rf "$dst"
mkdir -p "$dst"

for pkg in token lexer parser; do
  cp -R "$src/$pkg" "$dst/$pkg"
done

# Corpus: solo los .tape (la media del repo upstream no pinta nada aquí),
# preservando rutas relativas — los tests upstream leen
# ../examples/fixtures/all.tape y el runner de conformidad recorre todo.
(
  cd "$src"
  find examples -name '*.tape' -type f | while IFS= read -r f; do
    mkdir -p "$root/$dst/$(dirname "$f")"
    cp "$f" "$root/$dst/$f"
  done
)

cp "$src/LICENSE" "$dst/LICENSE"
# themes.json: los temas curados de VHS (Set Theme <nombre> debe migrar).
cp "$src/themes.json" "$dst/themes.json"

# Reescritura de imports: el único cambio sobre el código upstream.
find "$dst" -name '*.go' -exec perl -pi -e \
  's|github\.com/charmbracelet/vhs/|github.com/GH-Jaider/foley/tape/internal/vhsgrammar/|g' {} +

cat > "$dst/PROVENANCE.md" <<EOF
# Código vendoreado de VHS (no editar a mano)

- Origen: https://github.com/charmbracelet/vhs
- Release: $TAG (commit $COMMIT)
- Tarball sha256: $SHA256
- Licencia: MIT (LICENSE en este directorio)
- Cambios: únicamente la reescritura de import paths
  (github.com/charmbracelet/vhs/* → .../tape/internal/vhsgrammar/*).
- Regeneración: scripts/vendor-vhs.sh (ADR-008).
EOF

count=$(find "$dst" -name '*.tape' | wc -l | tr -d ' ')
echo "vendor-vhs: listo — token/lexer/parser + $count tapes de corpus"
