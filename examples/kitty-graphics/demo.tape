# The tape VHS cannot record: the demo transmits an image through the
# kitty graphics protocol and foley's embedded engine composites it into
# the frames. Plain VHS syntax throughout — run with `foley demo.tape`
# from this directory.
Output demo.gif

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
