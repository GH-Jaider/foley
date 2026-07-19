# The tape VHS cannot record: the demo transmits an image through the
# kitty graphics protocol and foley's embedded engine composites it
# into the frames, byte-exactly, on any machine. The richer takes live
# next door: lf/ (image previews in a file manager) and tenten/ (pixel
# art animating in realtime).

# No studio here on purpose: the take shows no paths and the e2e runs
# it EXACTLY as shipped (`foley demo.tape`, no flags).
Output demo.gif
# foley: dress foley

Set Width 640
Set Height 320
Set FontSize 18
Set TypingSpeed 45ms
Set WaitTimeout 20s

Sleep 400ms
Type "sh show.sh"
Enter
Wait+Screen /done/
Sleep 1.5s
