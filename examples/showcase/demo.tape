# Almost everything in one take: a dress, a custom prompt, the keys
# reel, a highlight and the camera — over a real git diff. Post-
# production only: the pty never knows any of it happened. This gif
# records at logical size for the README: foley -output-scale 1 demo.tape

Output showcase.gif
# foley: dress macos
# foley: keys small

Require git
Set Shell bash
Set Width 940
Set Height 520
Set FontSize 15
Set TypingSpeed 45ms

# The prompt is yours too (ADR-017): PS1 wins and bare Wait learns it.
Env PS1 "❯ "

# Deterministic git: pinned identity and dates.
Env GIT_AUTHOR_NAME "foley"
Env GIT_AUTHOR_EMAIL "demo@foley"
Env GIT_COMMITTER_NAME "foley"
Env GIT_COMMITTER_EMAIL "demo@foley"
Env GIT_AUTHOR_DATE "2026-01-01T00:00:00Z"
Env GIT_COMMITTER_DATE "2026-01-01T00:00:00Z"

Hide
Type@0ms "cd $(mktemp -d) && git init -q ."
Enter
Type@0ms "printf 'func handle() error {\n\treturn run(0)\n}\n' > retry.go && git add . && git commit -qm v1"
Enter
Type@0ms "printf 'func handle() error {\n\treturn run(3) // retries\n}\n' > retry.go"
Enter
Type@0ms "clear"
Enter
Show

Type "git diff --color"
Enter
Sleep 900ms
# foley: highlight /^\+.*retries/
Sleep 1200ms
# foley: zoom 0,3 53x13 700ms
Sleep 1800ms
# foley: zoom off 700ms
Sleep 400ms
# foley: highlight off
Sleep 900ms
