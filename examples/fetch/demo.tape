# The VHS-style fetch shot, the foley way: REAL fastfetch, no mockup —
# the film chip is the logo (foley.ans, the brand's single source) and
# the `Terminal: foley` line is GENUINE: foley IS the terminal the demo
# runs in (ADR-021); fastfetch just reports it.

Output fetch.gif
# foley: dress foley

Require fastfetch
Set Shell bash
Set Width 1000
Set Height 460
Set FontSize 15
Set Padding 30
Set TypingSpeed 35ms

Type "fastfetch --config foley.jsonc"
Enter
Sleep 5s
Screenshot fetch.png
