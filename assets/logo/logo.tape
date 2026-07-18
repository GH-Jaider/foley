# The logo IS a recording: foley types "foley" and these frames come
# out of the real engine — the film strip's screens are not drawings
# of a terminal, they are terminal. Record with:
#
#   cd assets/logo && foley -cols 8 -rows 3 logo.tape
#
# then compose the strip assets with tooling/logogen (make logo).
# Palette = the brand spec: screen #0D0B0A, ink #ECE6DF, REC #FF4F45.

Output logo-raw.gif
Require bash
Set Shell bash
Set Padding 0
Set FontSize 32
Set TypingSpeed 220ms
Set Theme {"name": "foley-logo", "background": "#0D0B0A", "foreground": "#ECE6DF", "cursor": "#FF4F45", "black": "#0D0B0A", "red": "#FF4F45", "green": "#ECE6DF", "yellow": "#ECE6DF", "blue": "#ECE6DF", "magenta": "#ECE6DF", "cyan": "#ECE6DF", "white": "#ECE6DF", "brightBlack": "#5A544E", "brightRed": "#FF4F45", "brightGreen": "#ECE6DF", "brightYellow": "#ECE6DF", "brightBlue": "#ECE6DF", "brightMagenta": "#ECE6DF", "brightCyan": "#ECE6DF", "brightWhite": "#ECE6DF"}
# The leading \n floats every prompt one row down: with 3 rows the
# line lands centered, no staging keystrokes needed.
Env PS1 "\n>"

# Let the launch prompt land before the first frame.
Sleep 300ms

# Type the name one frame at a time — each screenshot is one frame of
# the strip.
Screenshot logo-f0.png
Type@0ms "f"
Screenshot logo-f1.png
Type@0ms "o"
Screenshot logo-f2.png
Type@0ms "l"
Screenshot logo-f3.png
Type@0ms "e"
Screenshot logo-f4.png
Type@0ms "y"
Screenshot logo-f5.png
Sleep 900ms
