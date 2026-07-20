# The camera: push onto a region, hold, pull back — a 1:1 crop from
# the supersampled master, never an upscale.
Output zoom.gif
Require bash
Set Shell bash
Set Width 640
Set Height 300
Set FontSize 15
Set TypingSpeed 45ms
Set WaitTimeout 20s
# foley: dress ./foley-bare.dress.json
# foley: studio
Sleep 400ms
Type "cal 8 2026"
Enter
Wait
Sleep 700ms
# foley: zoom 0,1 40x9 600ms
Sleep 2000ms
# foley: zoom off 600ms
Sleep 700ms
