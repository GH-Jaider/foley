# The camera (ADR-019): push onto the line that matters, hold, pull
# back. The scene renders on a 2x supersampled master, so the zoom is
# a 1:1 crop — never an upscale, never blur. This gif records at
# logical size for the README: foley -output-scale 1 demo.tape Here: vim as the viewer,
# the camera as the reviewer's eye.

Output zoom.gif
# foley: dress macos

Require vim
Set Shell bash
Set Width 900
Set Height 480
Set FontSize 15
Set TypingSpeed 40ms

Hide
Type@0ms "cd $(mktemp -d)"
Enter
Type@0ms "cat > server.go <<'GO'"
Enter
Type@0ms "package main"
Enter
Type@0ms ""
Enter
Type@0ms "func handle(retries int) error {"
Enter
Type@0ms "	if retries < 0 { // BUG: zero disables the retry loop"
Enter
Type@0ms "		return errNoRetries"
Enter
Type@0ms "	}"
Enter
Type@0ms "	return runWithRetries(retries)"
Enter
Type@0ms "}"
Enter
Type@0ms "GO"
Enter
Type@0ms "clear"
Enter
Show

Type "vim -c 'set bg=dark nu' --clean server.go"
Enter
Sleep 1200ms
# foley: zoom 0,2 50x12 700ms
Sleep 1800ms
# foley: zoom off 700ms
Sleep 600ms
Hide
Type ":q!"
Enter
Show
Sleep 400ms
