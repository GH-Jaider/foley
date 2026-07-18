# The keys reel (ADR-016): every injected keystroke on a film strip
# under the window — keycap words (esc is esc), drawn arrows, a blank
# spacebar, coalescing repeats; the perforations are punched through
# to the margin fill. The canvas GROWS by the band: the footage is
# never covered. Knobs: small|medium|large, notation=keycap|icons,
# accent=<ansi|#hex|off>, plain.

Output keys.gif
# foley: dress macos
# foley: keys small

Set Shell bash
Set Width 860
Set Height 360
Set FontSize 15
Set TypingSpeed 90ms

Type "seq 1 3"
Enter
Sleep 700ms
Up
Sleep 400ms
Ctrl+E
Sleep 250ms
Backspace
Type "5"
Sleep 300ms
Enter
Sleep 1500ms
