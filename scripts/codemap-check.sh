#!/usr/bin/env bash
# Verifica que todo paquete Go y todo directorio top-level esté registrado en el
# inventario de CODEMAP.md (principio anti-slop 13: paquete sin mapa = CI rojo).
set -euo pipefail
cd "$(dirname "$0")/.."

map=CODEMAP.md
[[ -f $map ]] || { echo "codemap: falta $map" >&2; exit 1; }

# Solo la sección de inventario cuenta como registro (la tabla de ruteo no).
inv=$(awk '/^## Inventario/{f=1; next} /^## /{f=0} f' "$map")
fail=0

mod=$(go list -m)
while read -r pkg; do
  rel=${pkg#"$mod"}
  rel=${rel#/}
  [[ -z $rel ]] && rel="./"
  if ! grep -qF "\`$rel\`" <<<"$inv" && ! grep -qF "\`$rel/\`" <<<"$inv"; then
    echo "codemap: paquete sin registrar en CODEMAP.md → $rel" >&2
    fail=1
  fi
done < <(go list ./...)

for d in */; do
  d=${d%/}
  case $d in
    .git | .github) continue ;;
  esac
  if ! grep -qF "\`$d" <<<"$inv"; then
    echo "codemap: directorio top-level sin registrar → $d/" >&2
    fail=1
  fi
done

exit $fail
