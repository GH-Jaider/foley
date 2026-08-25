# The camera: push onto a region, hold, pull back. A 1:1 crop from
# the supersampled master, never an upscale.
Output zoom.gif
Require bash
Set Shell bash
Set Width 640
Set Height 360
Set FontSize 15
Set TypingSpeed 45ms
Set WaitTimeout 20s
# foley: dress ./foley-title.dress.json
# foley: studio
Sleep 400ms
Type "cal 8 2026"
Enter
Wait
Sleep 500ms
# foley: zoom 0,0 43x11 350ms
Sleep 1100ms
# foley: zoom off 350ms
Sleep 500ms
