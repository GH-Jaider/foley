# The keys reel on its motivating case: a TUI. lazygit's
# whole grammar is invisible keystrokes — j/k to move, space to stage,
# c to commit — and the film strip under the window is the only place
# they can be seen. Words as a real keycap prints them, a blank
# spacebar, drawn arrows; the perforations punch through to the
# margin. The canvas GROWS by the band: the footage is never covered.
# Knobs: small|medium|large, notation=keycap|icons, accent, plain.

Output keys.gif
# foley: dress foley
# foley: keys
# foley: studio

Require lazygit
Require git
Set Shell bash
Set Width 960
Set Height 540
Set FontSize 15
Set TypingSpeed 55ms

# Deterministic git: pinned identity and dates.
Env GIT_AUTHOR_NAME "foley"
Env GIT_AUTHOR_EMAIL "demo@foley"
Env GIT_COMMITTER_NAME "foley"
Env GIT_COMMITTER_EMAIL "demo@foley"
Env GIT_AUTHOR_DATE "2026-01-01T00:00:00Z"
Env GIT_COMMITTER_DATE "2026-01-01T00:00:00Z"

# The studio is the whole old ritual: the set is HOME and
# cwd, lazygit finds its config at the set's own XDG path, and
# everything is struck when the take ends.
Hide
Type@0ms "mkdir -p $XDG_CONFIG_HOME/lazygit && printf 'disableStartupPopups: true\nupdate:\n  method: never\ngui:\n  showRandomTip: false\n  showFileTree: false\n' > $XDG_CONFIG_HOME/lazygit/config.yml"
Enter
Type@0ms "mkdir retry && cd retry && git init -q ."
Enter
Type@0ms "printf 'func handle() error {\n\treturn run(0)\n}\n' > retry.go && printf 'max = 1\n' > retry.conf && git add . && git commit -qm 'retry: first pass'"
Enter
Type@0ms "printf 'func handle() error {\n\treturn run(3) // retries\n}\n' > retry.go && printf 'max = 3\n' > retry.conf"
Enter
Type@0ms "clear"
Enter
Show

Sleep 400ms
Type "lazygit"
Sleep 300ms
Enter
Sleep 1200ms

# Walk the files panel, stage retry.go with the invisible spacebar.
Type@250ms "jkj"
Sleep 500ms
Space
Sleep 900ms

# Commit: c opens the message, Enter seals it.
Type "c"
Sleep 900ms
Type "fix: enable retries"
Sleep 600ms
Enter
Sleep 2500ms
