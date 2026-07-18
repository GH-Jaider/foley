# The zoom cue (ADR-019): drive the camera. It eases onto a CELL rect
# (0-based, like highlight rects) and stays framed there until the next
# zoom or `zoom off`. The scene renders on a 2× master, so a full zoom
# is a 1:1 crop — never an upscale, never blur. One curve, no easing
# knob: the duration IS the shot. VHS ignores the cues and still records.

Output zoom.gif
# foley: dress macos

Set Width 700
Set Height 320
Set FontSize 16
Set TypingSpeed 40ms

Type "printf 'status: ok\nerror: missing flag\nhint: add --tag\n'"
Enter
Sleep 800ms
# foley: zoom 0,1 36x9 700ms
Sleep 1500ms
# foley: zoom off 700ms
Sleep 1s
