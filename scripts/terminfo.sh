#!/bin/sh
# Regenera internal/terminfo desde el PIN del motor: ghostty
# mantiene su terminfo como fuente Zig (src/terminfo/ghostty.zig) y la
# emite en build — aquí la emitimos desde el MISMO commit pineado en
# build.zig.zon y la compilamos con tic. Así la entrada que foley
# declara (TERM=xterm-ghostty) describe por construcción lo que el motor
# embebido implementa; un bump del pin exige re-correr esto y el diff de
# los artefactos viaja en el mismo commit.
#
# Herramientas de mantenedor (no de usuario): zig (el fetch verifica el
# hash del zon con el mismo algoritmo que el build de la .a) y tic
# (ncurses, presente en macOS y Linux base).
set -eu

cd "$(dirname "$0")/.."
root=$(pwd)
zon=$root/internal/vtengine/ghostty/libbuild/build.zig.zon
dst=$root/internal/terminfo

url=$(sed -n 's/^ *\.url = "\(.*\)",$/\1/p' "$zon")
hash=$(sed -n 's/^ *\.hash = "\(.*\)",$/\1/p' "$zon")
[ -n "$url" ] && [ -n "$hash" ] || {
  echo "terminfo: no pude leer url/hash de $zon" >&2
  exit 1
}

# zig fetch baja el paquete al cache global y devuelve su hash de
# contenido — la verificación contra el pin es exactamente la del build.
# (Corre desde libbuild/: zig 0.16 exige un build.zig como contexto.)
got=$(cd "$(dirname "$zon")" && zig fetch "$url")
if [ "$got" != "$hash" ]; then
  echo "terminfo: el paquete no coincide con el pin del zon:" >&2
  echo "  zon:   $hash" >&2
  echo "  fetch: $got" >&2
  exit 1
fi
cache=$(zig env | sed -n 's/^ *\.global_cache_dir = "\(.*\)",$/\1/p')

work=$(mktemp -d "${TMPDIR:-/tmp}/foley-terminfo.XXXXXX")
trap 'rm -rf "$work"' EXIT

# El cache de zig guarda el paquete como árbol (≤0.15) o como tarball
# (0.16) según versión — se acepta cualquiera de los dos.
if [ -f "$cache/p/$hash/src/terminfo/ghostty.zig" ]; then
  src=$cache/p/$hash/src/terminfo
elif [ -f "$cache/p/$hash.tar.gz" ]; then
  mkdir "$work/pkg"
  tar -xzf "$cache/p/$hash.tar.gz" -C "$work/pkg"
  src=$(find "$work/pkg" -type d -path '*/src/terminfo' | head -1)
else
  echo "terminfo: el paquete $hash no aparece en el cache de zig ($cache/p)" >&2
  exit 1
fi
[ -n "$src" ] && [ -f "$src/ghostty.zig" ] || {
  echo "terminfo: src/terminfo/ghostty.zig no está en el paquete del pin" >&2
  exit 1
}
cp "$src/ghostty.zig" "$src/Source.zig" "$work/"

# Emisor mínimo. Writer.fixed + debug.print (stderr) esquiva el churn
# del API de stdout entre versiones de zig; el encode es el del pin.
cat > "$work/emit.zig" <<'ZIG'
const std = @import("std");
const ghostty = @import("ghostty.zig").ghostty;

pub fn main() !void {
    var buf: [16384]u8 = undefined;
    var w = std.Io.Writer.fixed(&buf);
    try ghostty.encode(&w);
    std.debug.print("{s}", .{w.buffered()});
}
ZIG
(cd "$work" && zig run emit.zig 2> xterm-ghostty.terminfo)

grep -q '^xterm-ghostty|' "$work/xterm-ghostty.terminfo" || {
  echo "terminfo: lo emitido no empieza por xterm-ghostty| — ¿cambió la fuente del pin?" >&2
  exit 1
}

mkdir -p "$work/compiled"
tic -x -o "$work/compiled" "$work/xterm-ghostty.terminfo"
# tic escribe el layout del OS anfitrión (hex en macOS, letra en Linux)
# y hardlinkea los alias: un único blob. terminfo.Dir() lo materializa
# en runtime bajo los cuatro nombres que ncurses busca.
blob=$(find "$work/compiled" -type f -name xterm-ghostty)
[ -n "$blob" ] || {
  echo "terminfo: tic no produjo la entrada xterm-ghostty" >&2
  exit 1
}

cp "$work/xterm-ghostty.terminfo" "$dst/xterm-ghostty.terminfo"
cp "$blob" "$dst/xterm-ghostty"
printf 'terminfo: regenerado desde el pin %s\n' "$hash"
(cd "$dst" && ls -la xterm-ghostty xterm-ghostty.terminfo)
