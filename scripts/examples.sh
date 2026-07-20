#!/bin/sh
# Re-records every example gif from its tape with the real engine —
# the gifs ARE the documentation the README embeds, so after a visual
# change (a band redesign, a dress tweak) this is the one button.
# Explicit per-example blocks, not a clever loop: some takes carry
# flags (-output-scale 1 for the heavy gifs, -theme for the pair) and
# the table should read like a call sheet.
#
# Usage: scripts/examples.sh [example-dir ...]  (default: all)
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
FOLEY_FONTS="$root/internal/fontpack/fonts"
export FOLEY_FONTS

foley() {
	go run -tags ghosttyvt "$root/cmd/foley" "$@"
}

want() {
	# No args = record everything; otherwise only the named dirs.
	[ "$#" -eq 0 ] && set -- "$@" all
	case " $* " in
	*" all "*) return 0 ;;
	*" $current "*) return 0 ;;
	esac
	return 1
}

take() {
	printf '>> examples/%s\n' "$1"
	cd "$root/examples/$1"
	shift
	foley "$@"
}

current=dresses
if want "$@"; then
	for tape in "$root"/examples/dresses/*.tape; do
		take dresses "$(basename "$tape")"
	done
fi

current=fetch
want "$@" && take fetch demo.tape

current=highlight
want "$@" && take highlight demo.tape

current=keys
want "$@" && take keys demo.tape

current=kitty-graphics
want "$@" && take kitty-graphics demo.tape

# lf records inside the studio: the committed props reach
# the closed set through -env PROPS.
current=lf
want "$@" && take kitty-graphics/lf -env "PROPS=$root/examples/kitty-graphics/lf" demo.tape

current=pair
if want "$@"; then
	take pair -theme "Catppuccin Mocha" -o dark.gif demo.tape
	take pair -theme "Catppuccin Latte" -o light.gif demo.tape
fi

current=prompt
want "$@" && take prompt demo.tape

current=showcase
want "$@" && take showcase demo.tape

# tenten records in REALTIME: a continuous animation needs the wall
# clock (every other example runs deterministic).
current=tenten
want "$@" && take kitty-graphics/tenten -mode realtime demo.tape

current=zoom
want "$@" && take zoom demo.tape

# The README's mini-gifs (assets/readme): one feature per take, a few
# seconds each — the compact row the front page embeds; the narrative
# takes above stay the linked deep dives. The dress mini records twice
# on purpose: same tape, two looks.
current=readme
if want "$@"; then
	printf '>> assets/readme\n'
	cd "$root/assets/readme"
	foley -dress macos -o dress-macos.gif dress.tape
	foley -dress noir -o dress-noir.gif dress.tape
	foley keys.tape
	foley highlight.tape
	foley studio.tape
	foley title.tape
	foley zoom.tape
fi

printf 'examples: done\n'
