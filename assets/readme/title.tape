# The window bar is a real terminal's: an app declares its title
# (OSC 2) and the chrome follows it, live, on camera.
Output title.gif
Require bash
Set Shell bash
Set Width 640
Set Height 200
Set FontSize 15
Set TypingSpeed 45ms
Set WaitTimeout 20s
# foley: dress ./foley-title.dress.json
# foley: studio
Sleep 600ms
Type `printf '\033]2;vim notes.txt\033\\'`
Sleep 300ms
Enter
Sleep 1800ms
