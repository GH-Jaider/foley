# Pixel art LIVING in the terminal: tenten's own starter pet, animated
# over kitty graphics, recorded by foley in REALTIME mode — the one
# clock a continuous animation needs (the declarative clock captures
# state changes; a creature breathing needs the wall). Every other
# example runs deterministic; this one shows the other gear.
# Recorded with: foley -mode realtime demo.tape (see scripts/examples.sh)

Output tenten.gif
# foley: dress foley
# foley: studio

Require tenten
Set Shell bash
Set Width 840
Set Height 620
Set FontSize 15
Set Framerate 30
Set TypingSpeed 55ms

Hide
# tenten's own hello-world: scaffold the commented pet template. No
# TENTEN_PIXEL, no --pixel: tenten detects kitty graphics by terminal
# identity, and foley declares the one its engine implements —
# TERM=xterm-ghostty with its pinned terminfo shipped (ADR-021).
Type@0ms "tenten new critter --template pet >/dev/null"
Enter
Type@0ms "clear"
Enter
Show

Sleep 400ms
Type "tenten play critter.tenten.yml"
Sleep 300ms
Enter

# tenten sees a kitty-graphics terminal — foley declares the one its
# engine implements (TERM=xterm-ghostty, ADR-021) — and asks once per
# fresh HOME. Answered on camera: the question IS the demo.
Sleep 1200ms
Type "y"

# ~6 loops of the 810ms cycle, on the wall clock.
Sleep 5s
Type "q"
Sleep 600ms
