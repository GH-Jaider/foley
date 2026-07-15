#!/usr/bin/env bash
# Verifica que todo paquete Go y todo directorio top-level esté registrado en el
# inventario de CODEMAP.md (principio anti-slop 13), y que el inventario no
# tenga filas stale (rutas registradas que ya no existen).
#
# CODEMAP.md es PRIVADO (vive en .git/info/exclude, nunca se comitea): en un
# checkout sin él (CI, contribuidores) el check se omite con éxito — el gate
# es local por diseño.
set -euo pipefail
cd "$(dirname "$0")/.."

map=CODEMAP.md
if [[ ! -f $map ]]; then
  echo "codemap: $map no existe en este checkout (es privado) — check omitido" >&2
  exit 0
fi

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

# Filas stale: el primer token entre backticks de cada fila debe existir en
# disco. Filas marcadas "pendiente" están exentas (registran trabajo futuro).
while IFS= read -r line; do
  [[ $line == *pendiente* ]] && continue
  tok=$(sed -nE 's/^\| `([^`]+)`.*/\1/p' <<<"$line")
  [[ -z $tok ]] && continue
  tok=${tok%/}
  [[ $tok == "." || -e $tok ]] && continue
  echo "codemap: fila stale en CODEMAP.md (no existe en disco) → $tok" >&2
  fail=1
done <<<"$inv"

exit $fail
