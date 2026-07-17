#!/bin/sh
# Fetches the PINNED fontpack fonts (exact version + sha256).
# The canonical hashes live in internal/fontpack/fontpack.go (the package
# verifies them on every Load); this script must match them.
# POSIX sh: runs the same on macOS and CI Linux. Output is gitignored.
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
    echo "fonts: $name ok (cached)"
    return 0
  fi
  echo "fonts: fetching $name"
  curl -fsSL --retry 3 -o "$dst" "$url"
  got=$(sha256 "$dst")
  if [ "$got" != "$want" ]; then
    echo "fonts: HASH MISMATCH for $name" >&2
    echo "  expected: $want" >&2
    echo "  got:      $got" >&2
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
fetch JetBrainsMono-BoldItalic.ttf "$jb/JetBrainsMono-BoldItalic.ttf" \
  4039d5ce0ed225bf9c8b2c8c6436290ae2f356b7e90d70fa666227238324aa3b
fetch NotoColorEmoji.ttf "$noto/NotoColorEmoji.ttf" \
  39ee3c587e10e89669b9ff32703261d10d5f9c4dd5ad147b6b5a1c5200591817

# The by-NAME family catalog (ADR-015): popular terminal fonts,
# free licenses (OFL / UFL / Hack), immutable URLs
# (tag or commit SHA) and pinned sha256 — Set FontFamily "Fira Code"
# resolves against THIS catalog, never against the system.

# Fira Code 5.2 (OFL) — no italics: the italic slots degrade to weight.
fira=https://raw.githubusercontent.com/tonsky/FiraCode/5.2/distr/ttf
fetch FiraCode-Regular.ttf "$fira/FiraCode-Regular.ttf" \
  28c3ae21a853f1d74673384c7a0d620abb0e877b8c6cd8b64173a95512476824
fetch FiraCode-Bold.ttf "$fira/FiraCode-Bold.ttf" \
  37a609b7e27516ce0cf55cb7550edd1a1cbd8cd5bc028415a1d520c426c10357

# IBM Plex Mono v6.0.0 (OFL).
plex=https://raw.githubusercontent.com/IBM/plex/v6.0.0/IBM-Plex-Mono/fonts/complete/ttf
fetch IBMPlexMono-Regular.ttf "$plex/IBMPlexMono-Regular.ttf" \
  a3c50f7c0e063998cfaeae56c6169ece9e0feaffaef425aa038f85d037fb4b9b
fetch IBMPlexMono-Bold.ttf "$plex/IBMPlexMono-Bold.ttf" \
  5474dd5d5c3dc6c027cac93fe7e5a736e7b33adb4717093a1e23b36aab4606e9
fetch IBMPlexMono-Italic.ttf "$plex/IBMPlexMono-Italic.ttf" \
  d70bd62fa6b97d19853c0cf823667f99f7ff023d915052248e68635179c8fa83
fetch IBMPlexMono-BoldItalic.ttf "$plex/IBMPlexMono-BoldItalic.ttf" \
  3d0c0888a9c3a98b39fc5aace9c20b149c793063cd9e9e0634f561e55186c4bf

# Source Code Pro (OFL) — commit SHA of the release branch (immutable).
scp=https://raw.githubusercontent.com/adobe-fonts/source-code-pro/803b7e23ec97ae58b6232ea76519a76d428ba268/TTF
fetch SourceCodePro-Regular.ttf "$scp/SourceCodePro-Regular.ttf" \
  74bd80d3e42a08517cd7e1108ba3d86f2da29ac0f3065be95e0357956ab9db37
fetch SourceCodePro-Bold.ttf "$scp/SourceCodePro-Bold.ttf" \
  b2095e0d657e6d28dc32444a9dacabab0c9241d0bf39d96371756cc9bdbc3a5f
fetch SourceCodePro-It.ttf "$scp/SourceCodePro-It.ttf" \
  9c9e0f4d016210a3c5bdfba5262637c5b26ddff4ccc382ebbc781de5961d0042
fetch SourceCodePro-BoldIt.ttf "$scp/SourceCodePro-BoldIt.ttf" \
  1b49d9304012bf8db9e5dd4104183d5c122c445d0570a2259125f71977595b90

# Hack v3.003 (MIT + Bitstream Vera).
hack=https://raw.githubusercontent.com/source-foundry/Hack/v3.003/build/ttf
fetch Hack-Regular.ttf "$hack/Hack-Regular.ttf" \
  15f55cc0c85a2988d2b4b3a8cdb5d77fdfbaf319e1bb5309d725db9818fb7125
fetch Hack-Bold.ttf "$hack/Hack-Bold.ttf" \
  5bbf531eff7f8a0c2559c9a0656718e2828a012a9b1f60b5f54006d59a4de8d4
fetch Hack-Italic.ttf "$hack/Hack-Italic.ttf" \
  096fb67a2b85f3c866e9cb3e965b27c2c10b977315f4d3d7f095674be35091c1
fetch Hack-BoldItalic.ttf "$hack/Hack-BoldItalic.ttf" \
  64f74a079700b7dfe128551a1e28875d5ba980971e55f5e0f0596e37bdc6a6bc

# Ubuntu Mono (UFL) — commit SHA of google/fonts (immutable).
ubuntu=https://raw.githubusercontent.com/google/fonts/389b770410cc0b7c21c85673bfa2077420fe7f65/ufl/ubuntumono
fetch UbuntuMono-Regular.ttf "$ubuntu/UbuntuMono-Regular.ttf" \
  b35dd9d2131d5d83a9b87fe9ad22c6288fa3d17688d43302c14da29812417d63
fetch UbuntuMono-Bold.ttf "$ubuntu/UbuntuMono-Bold.ttf" \
  11f15c3a6bbd998a8695fdefb3475931c3789aa035d7546f2efe78e83b352f6b
fetch UbuntuMono-Italic.ttf "$ubuntu/UbuntuMono-Italic.ttf" \
  960b2bc286c2ff7d49073303858c65e1fc9013c17a971b61123b02c39454ef75
fetch UbuntuMono-BoldItalic.ttf "$ubuntu/UbuntuMono-BoldItalic.ttf" \
  bd255784bb87b5c41513a12a86f0f9cf061bce4e8256d3bfe7234611002e8f48

echo "fonts: done"
