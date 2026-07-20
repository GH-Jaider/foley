# shellcheck shell=bash
# shellcheck disable=SC1003 # the \\ in printf is the ST terminator, not a quote escape
# cast.sh — the showcase's hidden crew (sourced off camera).
# A gif is a silent film; foley is the craft of adding sound. So the
# sound effects are WRITTEN — and each card confesses HOW the sound
# was made, the way a foley artist would: a coat for the curtain, a
# fistful of spaghetti for the roses, one slate clap looped for the
# applause. Every scene the tape can't type lives here as one ASCII
# word — the pty only ever types plain names, the unicode stays here.

# _geom — the set's true size, read from the pty itself (stty), never
# terminfo: where center is must not depend on the studio's TERM.
_geom() {
	_sz=$(stty size 2>/dev/null) || _sz='26 116'
	_lines=${_sz% *}
	_cols=${_sz#* }
}

# _cardb BORDERCOLOR LINE... — an intertitle, centered on its own
# black; an empty color means the house border. _card is the plain form.
_cardb() {
	_bd=$1
	shift
	clear
	_geom
	_pad=$(((_lines - 7) / 2))
	_i=0
	while [ "$_i" -lt "$_pad" ]; do
		printf '\n'
		_i=$((_i + 1))
	done
	if [ -n "$_bd" ]; then
		gum style --border rounded --border-foreground "$_bd" --align center \
			--width 50 --padding "1 4" --margin "0 $(((_cols - 52) / 2))" "$@"
	else
		gum style --border rounded --align center \
			--width 50 --padding "1 4" --margin "0 $(((_cols - 52) / 2))" "$@"
	fi
}

_card() { _cardb '' "$@"; }

# _slug TITLE — the scene heading, spoken through the window bar (OSC 2).
_slug() { printf '\033]2;%s\033\\' "$1"; }

# _dossier — the secret, typeset BIG (cfonts tiny) in vault green and
# written to the set's cwd: the decrypt gets letters worth a close-up,
# so the scene needs no camera at all.
_dossier() {
	{
		cfonts 'NO WINDOW.' -f tiny -a center -g "#39ff5a,#b0ff66" -t
		cfonts 'NO CAPTURE.' -f tiny -a center -g "#39ff5a,#b0ff66" -t
		cfonts 'NO PERMISSIONS.' -f tiny -a center -g "#39ff5a,#b0ff66" -t
	} >dossier.txt
}

_hush() { printf '\033[?25l'; }
_speak() { printf '\033[?25h'; }

# _marquito ROW COL — a 14x7 rounded frame for the director's photo.
_marquito() {
	_mr=$1
	_mc=$2
	printf '\033[%d;%dH╭────────────╮' "$_mr" "$_mc"
	for _mk in 1 2 3 4 5; do
		printf '\033[%d;%dH│            │' "$((_mr + _mk))" "$_mc"
	done
	printf '\033[%d;%dH╰────────────╯' "$((_mr + 6))" "$_mc"
}

# _pfp ROW COL — the director's own photo through the kitty protocol
# (a=T f=100, c/r: the PNG stretched into a 12x5 cell rect at the
# cursor) — the credits are shot on the same camera as the premiere.
_pfp() {
	printf '\033[%d;%dH' "$1" "$2"
	printf '\033_Gf=100,a=T,c=12,r=5;%s\033\\' "$(base64 <"$PROPS/pfp.png" | tr -d '\n')"
}

# _rollat ROW COL LINE — one credit, typed into place.
_rollat() {
	printf '\033[%d;%dH%s' "$1" "$2" "$3"
	sleep 0.75
}

# coldopen — the rating band, the production chip wiped in (the A24
# beat), the slate. The chip arrives via $LOGO (assets/logo): brand
# assets are never duplicated into props. Canvas 0 = the full
# terminal, so anchor-text centers the chip; --no-eol keeps tte's
# exit from scrolling the held chip up a row.
coldopen() {
	_hush
	_cardb '#ff4f45' 'THE FOLLOWING PREVIEW HAS BEEN' 'APPROVED FOR ALL TERMINALS' '' 'THE TAPE ADVERTISED HAS BEEN RATED PTY'
	sleep 2.6
	_speak
	clear
	tte --input-file "$LOGO/foley.ans" --existing-color-handling dynamic \
		--canvas-width 0 --canvas-height 0 --anchor-text c --no-eol wipe
	_hush
	sleep 1.3
	_card '🎬  FOLEY presents' '«THE SHOWCASE»'
	sleep 1.8
	_card '[ CLAP ]' '( two hands, both mine )'
	sleep 1.1
	_slug 'SC 1 — THE TITLE'
	_speak
	clear
}

# voice — the trailer horn, made the foley way.
voice() {
	_hush
	_card '[ BRAAAM ]' '( mouth trumpet, one take )'
	sleep 2.2
	_slug 'SC 2 — INT. MAINFRAME — NIGHT'
	_speak
	clear
}

# interlude — the curtain is a winter coat; someone drags it.
interlude() {
	_hush
	_card '[ CURTAIN ]' '( a winter coat, dragged slowly )'
	sleep 1.7
	_slug 'SC 3 — THE PREMIERE'
	_speak
	clear
}

# finale — roses (a fistful of spaghetti), the FIN card, the credit
# roll beside the director's photo, the fine print, out on black. The
# cursor returns for the very last black: the loop hands it back to
# frame one, where it waits.
finale() {
	_hush
	_card '[ ROSES HIT THE STAGE ]' '( a fistful of spaghetti, dropped )'
	sleep 1.7
	clear
	_geom
	_pad=$(((_lines - 17) / 2))
	_i=0
	while [ "$_i" -lt "$_pad" ]; do
		printf '\n'
		_i=$((_i + 1))
	done
	cfonts FIN -f block --align center --gradient "#ff4f45,#ffd700" --transition-gradient
	sleep 1.2
	_top=$((_pad + 9))
	_left=$(((_cols - 65) / 2))
	_marquito "$_top" "$_left"
	_pfp "$((_top + 1))" "$((_left + 1))"
	sleep 1.0
	_ct=$((_left + 17))
	_rollat "$((_top + 1))" "$_ct" 'directed by ........ foley.tape'
	_rollat "$((_top + 2))" "$_ct" 'shot on ............ libghostty-vt'
	_rollat "$((_top + 3))" "$_ct" 'a .................. hithere (jaider) production'
	sleep 0.3
	_rollat "$((_top + 5))" "$_ct" '[ APPLAUSE ] ( the slate clap, looped )'
	sleep 0.6
	printf '\033[%d;%dH\033[2mno terminals were opened in the making of this gif\033[0m' \
		"$((_top + 8))" "$(((_cols - 50) / 2))"
	printf '\033[%d;1H' "$((_top + 9))"
	sleep 2.8
	clear
	_speak
	sleep 3.0
}
