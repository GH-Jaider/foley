# lf with IMAGE previews — the shot VHS cannot take: the photo enters
# the terminal through the kitty graphics protocol (preview.sh emits
# the bytes, clean.sh deletes the placement) and foley records the
# placements natively, deterministic like everything else. Navigation
# reads in the keys reel: j/k between text and image and back.

Output lf.gif
# foley: dress paper
# foley: keys small
# foley: studio

Require lf
Set Shell bash
Set Width 960
Set Height 480
Set FontSize 15
Set TypingSpeed 55ms

Hide
# The studio (ADR-023) is the set; the example's committed props
# arrive via -env PROPS (see scripts/examples.sh). lf's config goes
# through -config EXPLICITLY (its per-OS search paths ignore a set
# HOME on macOS) behind an alias, so the visible command stays a
# clean `lf`. promptfmt drops user@host: the hostname comes from the
# kernel, past the studio's env — the app's own knob covers it.
Type@0ms `printf 'set previewer %s/preview.sh\nset cleaner %s/clean.sh\nset drawbox true\nset promptfmt "\\033[1m%%f\\033[0m"\n' "$PROPS" "$PROPS" > $XDG_CONFIG_HOME/lfrc && alias lf='lf -config $XDG_CONFIG_HOME/lfrc'`
Enter
Type@0ms "mkdir frieren && cd frieren && cp $PROPS/frieren.png ."
Enter
Type@0ms "printf '# frieren\n\nEl set de este demo: una foto, un go, un md.\n' > README.md"
Enter
Type@0ms "printf 'package retry\n\nfunc Handle() error {\n\treturn run(3)\n}\n' > retry.go"
Enter
Type@0ms "touch -t 202601010000 * && clear"
Enter
Show

Sleep 400ms
Type "lf"
Sleep 300ms
Enter
Sleep 700ms
# Off camera: lf's startup clear lands AFTER the first preview already
# transmitted, eating the placement (ED deletes kitty images). A
# hidden reselect re-places it once the screen is stable.
Hide
Type@0ms "jk"
Sleep 400ms
Show
# The cursor opens on the photo (first by sort): kitty pixels, framed.
Sleep 1600ms
# Down through the text neighbors…
Type "j"
Sleep 1300ms
Type "j"
Sleep 1100ms
# …and back to her.
Type "k"
Sleep 600ms
Type "k"
Sleep 2200ms
Type "q"
Sleep 700ms
