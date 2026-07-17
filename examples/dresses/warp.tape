# One scene, four looks. The tape is the recording; the dress is
# post-production — VHS ignores the cue below and still records this
# tape, just undressed. Swap looks without editing:
#
#   foley warp.tape
#   foley -dress iterm warp.tape      (override the layer)
#   foley wardrobe                  (list the looks)

Output warp.gif
# foley: dress warp

Set Width 700
Set Height 400
Set FontSize 16
Set TypingSpeed 35ms

Type "echo 'same tape, dressed as: warp'"
Enter
Sleep 1200ms
Type "seq 1 3"
Enter
Sleep 2s
