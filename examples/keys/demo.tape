# The keys band (ADR-016): foley IS the one typing, so the input track
# is emitted, not captured — one keycap per keystroke, repeats
# coalesce (a ×3), chords are one cap (⌃C), phrases cut like
# subtitles. VHS ignores both cues and still records this tape.

Output keys.gif
# foley: dress macos
# foley: keys

Set Width 700
Set Height 240
Set FontSize 16
Set TypingSpeed 90ms

Type "echo holaaa"
Sleep 400ms
Enter
Sleep 1s
Up
Sleep 600ms
Ctrl+C
Sleep 2500ms
