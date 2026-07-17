# One scene, a whole wardrobe — and this one is the point of a dress: a
# BRAND KIT. The dress file carries palette + type + chrome, ships with
# the repo, and every tape wears it with this single cue line. Note
# there is no Set FontSize here: the dress supplies it.

Output brand.gif
# foley: dress ./brand.dress.json

Set Width 700
Set Height 340
Set TypingSpeed 35ms

Type "echo 'same tape, dressed as: brand kit'"
Enter
Sleep 1200ms
Type "seq 1 3"
Enter
Sleep 2s
