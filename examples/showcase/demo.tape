# The showcase, shot as what it is: a trailer. A gif is a silent
# film, and foley is the craft that adds the sound — so every sound
# effect here is WRITTEN, and each card confesses how it was made
# (mouth trumpet, dried pasta, one clap looped): the foley artist's
# credit sheet. Four commands land on camera and the reel narrates
# them; the rest is a hidden crew (props/cast.sh — the pty only ever
# TYPES plain ASCII scene names; the unicode stays sourced, never
# keyed). The premiere is maru-maga, a real tenten film, playing in
# kitty pixels on a set dressed for it (setup plants tenten's consent
# in the studio's own XDG); the credits put the director's photo on
# screen through the same protocol. Realtime is the one clock the
# animations need. Recorded with:
#   foley -mode realtime -env PROPS=… -env LOGO=… demo.tape

Output showcase.gif
# foley: dress foley
# foley: keys small
# foley: studio

Require tte
Require cfonts
Require gum
Require tenten

Set Shell bash
Set Width 940
Set Height 520
Set FontSize 15
Set Framerate 30
Set TypingSpeed 60ms
Set WaitTimeout 45s

Env PS1 "ACTION ❯ "

# -- setup (off camera): source the crew, stage the props, dress the
# set for pixels (tenten's consent, planted like lf's lfrc).
Hide
Type@0ms `. "$PROPS/cast.sh" && cp "$PROPS/maru-maga.tenten.yml" . && _dossier && CFG="${XDG_CONFIG_HOME:-$HOME/.config}/tenten" && mkdir -p "$CFG" && printf '{"pixel":true,"sound":false}\n' > "$CFG/config.json" && clear`
Enter
Show
Sleep 500ms

# -- COLD OPEN: the green band, the production chip, then the slate
Hide
Type@0ms "coldopen"
Enter
Sleep 250ms
Show
Wait
Sleep 350ms

# -- SC 1: the title — cfonts through tte's beams, centered on the
# full canvas, colors preserved
Type@18ms `cfonts FOLEY -f block -g "#ff4f45,#ffd700" -t | tte --existing-color-handling dynamic --canvas-width 0 --canvas-height 0 --anchor-text c beams --beam-delay 5 --final-wipe-speed 2`
Sleep 300ms
Enter
Wait
Sleep 1100ms

# -- the trailer horn, made the foley way
Hide
Type@0ms "voice"
Enter
Sleep 250ms
Show
Wait
Sleep 350ms

# -- SC 2: the heist — the dossier decrypts in BIG type (cfonts,
# typeset by the crew at setup): letters worth the close-up, so the
# scene needs no camera. The highlight lands by rect: the words on
# screen are half-block art, not text a regex could find.
Type@18ms `tte -i dossier.txt --existing-color-handling dynamic decrypt --typing-speed 6`
Sleep 300ms
Enter
Wait
Sleep 500ms
# foley: highlight 26,8 56x4
Sleep 2000ms
# foley: highlight off
Sleep 700ms

# -- intermission
Hide
Type@0ms "interlude"
Enter
Sleep 250ms
Show
Wait
Sleep 350ms

# -- SC 3: the premiere — maru-maga, a real tenten film, living in
# kitty pixels: act one in the studio (the hat, the wand), the camera
# pushes onto the stage, pulls back for the teleport, and the trailer
# cuts on the reveal of the field. A trailer teases; it never resolves.
Type "tenten play maru-maga.tenten.yml"
Sleep 300ms
Enter
Sleep 3800ms
# foley: zoom 17,2 56x17 900ms
Sleep 2600ms
# foley: zoom off 900ms
Sleep 5900ms
# the director calls cut — the q lands on the reel, honestly
Type "q"
Sleep 500ms

# -- FIN: roses (dried pasta), the card, the credits beside the
# director's photo — kitty graphics again — and out on black
Hide
Type@0ms "finale"
Enter
Sleep 250ms
Show
Sleep 13500ms
Hide
Sleep 300ms
