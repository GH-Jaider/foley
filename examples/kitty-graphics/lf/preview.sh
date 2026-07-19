#!/bin/sh
# lf previewer: plain text via cat; PNG images travel the kitty
# graphics protocol as DATA at the preview pane's cells — the medium
# an embedded terminal can actually record (a file path would ask the
# terminal to read the disk; foley's engine, like any pty-embedded VT,
# only sees bytes on the wire).
# lf calls it as: preview.sh FILE WIDTH HEIGHT X Y (pane cells, 0-based)
set -eu
case "$1" in
*.png)
	b64=$(base64 <"$1" | tr -d '\n')
	# Fit the (square) photo inside the pane, explicit c AND r — the
	# cell aspect is the demo's pinned metrics (9x20 logical at
	# FontSize 15). Both axes matter beyond aspect: r-less placements
	# lose their anchor in the embedded engine (probe-confirmed).
	r=$(($3 - 1))
	c=$((r * 20 / 9))
	if [ "$c" -gt "$(($2 - 2))" ]; then
		c=$(($2 - 2))
		r=$((c * 9 / 20))
	fi
	# ONE write: save cursor, jump to the pane, transmit, restore.
	# Atomicity matters — lf keeps painting on the same tty. Straight
	# to the tty: lf treats stdout as pane text.
	# shellcheck disable=SC1003 # the trailing \\ is printf for backslash: ESC \ is the APC terminator (ST), not a quote escape
	printf '\0337\033[%d;%dH\033_Gf=100,a=T,c=%d,r=%d;%s\033\\\0338' \
		"$(($5 + 1))" "$(($4 + 1))" "$c" "$r" "$b64" >/dev/tty
	# Volatile (exit 1): the placement is not lf's to cache — the
	# cleaner deletes it and each selection re-places.
	exit 1
	;;
*)
	cat "$1"
	;;
esac
