# The highlight cue (ADR-018): point the viewer's eye. It paints the
# theme's Selection color under a /regex/ from its position in the
# script until `off` — pure post-production, the terminal never knows.

Output highlight.gif
# foley: dress noir
# foley: studio

Set Shell bash
Set Width 860
Set Height 380
Set FontSize 15
Set TypingSpeed 35ms

Hide
Type@0ms "cat > run-tests.sh <<'SH'"
Enter
Type@0ms "#!/bin/sh"
Enter
Type@0ms "printf 'ok   api      412 passed  0.31s\n'"
Enter
Type@0ms "printf 'ok   auth     198 passed  0.22s\n'"
Enter
Type@0ms "printf 'FAIL store    check_retry: want 3, got 0\n'"
Enter
Type@0ms "printf 'ok   render    87 passed  0.09s\n'"
Enter
Type@0ms "SH"
Enter
Type@0ms "chmod +x run-tests.sh && clear"
Enter
Show

Type "./run-tests.sh"
Enter
Sleep 900ms
# foley: highlight /FAIL.*/
Sleep 2200ms
# foley: highlight off
Sleep 800ms
