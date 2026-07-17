# A custom prompt (ADR-017): Env PS1 was always legal grammar — foley
# makes it WIN over the pinned shell table, and a bare Wait learns the
# new prompt (its pattern derives from PS1, loudly when it cannot).
# This tape still records in VHS too — there the Env has no effect and
# it degrades to VHS's own prompt.

Output prompt.gif
# foley: dress macos

Set Width 700
Set Height 230
Set FontSize 16
Set TypingSpeed 45ms
Set WaitTimeout 20s
Env PS1 "\[\e[35m\]❯\[\e[0m\] "

Sleep 400ms
Type "echo mi prompt, mis reglas"
Enter
Wait
Sleep 2s
