# Almost everything in one take, as a story: find the file with fzf,
# read it with bat, spot the bug — dress, custom prompt, the keys
# reel, a highlight and the camera, over real tools. Post-production
# only: the pty never knows any of it happened.

Output showcase.gif
# foley: dress foley
# foley: keys small
# foley: studio

Require fzf
Require bat
Set Shell bash
Set Width 940
Set Height 520
Set FontSize 15
Set TypingSpeed 70ms

# The prompt is yours too: PS1 wins and bare Wait learns it.
Env PS1 "❯ "
# A pinned walker: fzf's default find order is filesystem-dependent.
Env FZF_DEFAULT_COMMAND "ls -1"

# The studio is the set: files land on a fresh stage, struck
# after the take.
Hide
Type@0ms "printf 'package retry\n\nfunc Handle() error {\n\treturn run(0) // FIXME: zero disables the retry loop\n}\n' > retry.go"
Enter
Type@0ms "printf 'package retry\n\nfunc run(n int) error {\n\treturn nil\n}\n' > run.go"
Enter
Type@0ms "printf 'package retry\n\ntype Config struct {\n\tMax int\n}\n' > config.go"
Enter
Type@0ms "clear"
Enter
Show

Sleep 500ms
Type `bat --style=header,numbers --theme=ansi "$(fzf --reverse --height=6)"`
Sleep 400ms
Enter
Sleep 700ms
Type "ret"
Sleep 900ms
Enter
# bat prints AFTER fzf's cleanup settles — wait for the prompt so the
# cues below act on the painted file, not on a blank between-frame.
Wait
Sleep 800ms
# foley: highlight /return run\(0\).*/
Sleep 1400ms
# foley: zoom 0,2 62x7 700ms
Sleep 2000ms
# foley: zoom off 700ms
Sleep 600ms
# foley: highlight off
Sleep 1200ms
