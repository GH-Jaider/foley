# The highlight cue (ADR-018): point the viewer's eye. It paints the
# theme's Selection color under a /regex/ (or a COL,ROW WxH cell rect)
# from its position in the script until `off` — pure post-production,
# the terminal never knows. VHS ignores the cues and still records.

Output highlight.gif
# foley: dress macos

Set Width 700
Set Height 260
Set FontSize 16
Set TypingSpeed 40ms

Type "printf 'ok: build\ndone: tests\nerror: missing flag\n'"
Enter
Sleep 800ms
# foley: highlight /^error.*/
Sleep 2s
# foley: highlight off
Sleep 1s
