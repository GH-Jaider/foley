# The input reel: foley is the one typing, so every key lands on the
# strip with its exact timing — recall and chords included.
Output keys.gif
Require bash
Set Shell bash
Set Width 640
Set Height 230
Set FontSize 15
Set TypingSpeed 95ms
Set WaitTimeout 20s
# foley: dress ./foley-bare.dress.json
# foley: keys
# foley: studio
Sleep 400ms
Type "seq 3"
Sleep 250ms
Enter
Wait
Sleep 600ms
Up
Sleep 500ms
Ctrl+C
Sleep 1300ms
