#!/bin/sh
# Vendors VHS's REAL .tape grammar (ADR-008), pinned by release.
#
# Brings token/, lexer/, parser/ (code + tests), the example corpus
# (*.tape only) and the LICENSE, rewriting import paths into foley's
# tree. The rewrite is the ONLY allowed change. Never edit
# tape/internal/vhsgrammar/ by hand — it regenerates from the pin.
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

echo "vendor-vhs: fetching vhs $TAG"
curl -fsSL --retry 3 -o "$tmp/vhs.tar.gz" \
  "https://github.com/charmbracelet/vhs/archive/refs/tags/$TAG.tar.gz"
got=$(sha256 "$tmp/vhs.tar.gz")
if [ "$got" != "$SHA256" ]; then
  echo "vendor-vhs: tarball HASH MISMATCH" >&2
  echo "  expected: $SHA256" >&2
  echo "  got:      $got" >&2
  exit 1
fi
tar -xzf "$tmp/vhs.tar.gz" -C "$tmp"
src=$tmp/vhs-${TAG#v}

rm -rf "$dst"
mkdir -p "$dst"

for pkg in token lexer parser; do
  cp -R "$src/$pkg" "$dst/$pkg"
done

# Corpus: .tape files only (upstream's media is dead weight here),
# preserving relative paths — upstream tests read
# ../examples/fixtures/all.tape and the conformance runner walks it all.
(
  cd "$src"
  find examples -name '*.tape' -type f | while IFS= read -r f; do
    mkdir -p "$root/$dst/$(dirname "$f")"
    cp "$f" "$root/$dst/$f"
  done
)

cp "$src/LICENSE" "$dst/LICENSE"
# themes.json: VHS's curated themes (Set Theme <name> must migrate).
cp "$src/themes.json" "$dst/themes.json"

# Import rewrite: the only change over upstream code.
find "$dst" -name '*.go' -exec perl -pi -e \
  's|github\.com/charmbracelet/vhs/|github.com/GH-Jaider/foley/tape/internal/vhsgrammar/|g' {} +

cat > "$dst/PROVENANCE.md" <<EOF
# Code vendored from VHS (do not edit by hand)

- Origin: https://github.com/charmbracelet/vhs
- Release: $TAG (commit $COMMIT)
- Tarball sha256: $SHA256
- License: MIT (LICENSE in this directory)
- Changes: import-path rewrite only
  (github.com/charmbracelet/vhs/* -> .../tape/internal/vhsgrammar/*).
- Regenerate with: scripts/vendor-vhs.sh (ADR-008).
EOF

count=$(find "$dst" -name '*.tape' | wc -l | tr -d ' ')
echo "vendor-vhs: done — token/lexer/parser + $count corpus tapes"
