# The camera earning its keep: tmux splits the screen, the
# right pane runs the tests, and the FAIL is too small to read — so
# the camera pushes onto THAT PANE, holds, and pulls back. The scene
# renders on a 2x supersampled master: the zoom is a 1:1 crop, never
# an upscale, never blur. The keys reel narrates the tmux chord.

Output zoom.gif
# noir, plus the reactive title: the bar starts as "~" and
# becomes "tmux" the moment tmux declares itself via OSC 2 — like a
# real terminal's tab. Footage, not host state.
# foley: dress {"theme":"TokyoNight","windowBar":"Colorful","windowBarSize":28,"windowTitle":"~","borderRadius":10,"padding":30,"margin":36,"marginFill":"#0d0e16","windowTitleFollow":true}
# foley: keys small
# foley: studio

Require tmux
Require bat
Set Shell bash
Set Width 1000
Set Height 500
Set FontSize 15
Set TypingSpeed 55ms

Hide
# The studio is the set: tmux reads the take's own
# ~/.tmux.conf (no status clock — the recording must not tick; the
# default status-right even carries the HOSTNAME) and set-titles makes
# it DECLARE its title over OSC 2 (the string is pinned: the default
# leaks pane titles and hostnames). TMUX_TMPDIR is the one tmux-ism
# the studio's TMPDIR does not cover: without it the SERVER lands on
# the host's socket and outlives the take. Panes do not inherit PS1
# through tmux, so a tiny rcfile pins the same prompt the outer shell
# wears (%q survives the color escapes).
Type@0ms `export TMUX_TMPDIR=$TMPDIR && printf 'PS1=%q\n' "$PS1" > ~/.panerc`
Enter
Type@0ms `printf 'set -g status-right ""\nset -g default-command "bash --rcfile ~/.panerc --noprofile"\nset -g set-titles on\nset -g set-titles-string "tmux"\n' > ~/.tmux.conf`
Enter
Type@0ms "printf 'func handle() error {\n\treturn run(0) // FIXME: retries off\n}\n' > retry.go"
Enter
Type@0ms "cat > test.sh <<'SH'"
Enter
Type@0ms "printf '\033[1mgo test ./retry\033[0m\n'"
Enter
Type@0ms "printf -- '--- \033[31mFAIL\033[0m: TestHandleRetries (0.00s)\n'"
Enter
Type@0ms "printf '    retry_test.go:14: run(0): retries disabled,\n'"
Enter
Type@0ms "printf '        want 3 attempts before giving up\n'"
Enter
Type@0ms "printf '\033[31mFAIL\033[0m\nexit status 1\n'"
Enter
Type@0ms "SH"
Enter
Type@0ms "clear"
Enter
Show

Sleep 400ms
Type "tmux"
Sleep 300ms
Enter
Sleep 1100ms

Type "bat --style=numbers --theme=ansi retry.go"
Sleep 300ms
Enter
Sleep 1000ms

# The tmux chord, narrated by the reel: prefix, then the split.
Ctrl+B
Sleep 450ms
Type "%"
Sleep 1100ms

Type "sh test.sh"
Sleep 300ms
Enter
Sleep 1200ms

# Push onto the right pane: the FAIL becomes readable.
# foley: zoom 40,0 56x13 700ms
Sleep 2400ms
# foley: zoom off 700ms
Sleep 1200ms

# Epilogue, off camera: the private tmux server dies with the take —
# no orphan process survives the render.
Hide
Type@0ms "tmux kill-server"
Enter
