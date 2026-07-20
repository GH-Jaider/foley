# The closed set: HOME, the working directory and every temp default
# live on a fresh stage, struck when the take ends — your machine
# never makes it on camera.
Output studio.gif
Require bash
Set Shell bash
Set Width 640
Set Height 220
Set FontSize 15
Set TypingSpeed 45ms
Set WaitTimeout 20s
# foley: dress ./foley-bare.dress.json
# foley: studio
Sleep 400ms
Type "echo $USER@$HOSTNAME"
Enter
Wait
Sleep 500ms
Type "ls -A ~"
Enter
Wait
Sleep 1600ms
