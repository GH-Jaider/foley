# One scene, four looks. The tape is the recording; the dress is
# post-production — VHS ignores the cue below and still records this
# tape, just undressed. Swap looks without editing:
#
#   foley bare.tape
#   foley -dress iterm bare.tape      (override the layer)
#   foley wardrobe                  (list the looks)

Output bare.gif
# foley: dress bare

Set Width 700
Set Height 400
Set FontSize 16
Set TypingSpeed 35ms

Type "echo 'same tape, dressed as: bare'"
Enter
Sleep 1200ms
Type "seq 1 3"
Enter
Sleep 2s
