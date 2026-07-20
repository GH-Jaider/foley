# The viewer's eye: a band of the theme's own Selection color, from
# this point in the script until off.
Output highlight.gif
Require bash
Set Shell bash
Set Width 640
Set Height 230
Set FontSize 15
Set TypingSpeed 45ms
Set WaitTimeout 20s
# foley: dress ./foley-bare.dress.json
# foley: studio
Hide
Type@0ms `printf 'ok    auth      41ms\nok    cache      9ms\nFAIL  retry — want 3 attempts, got 1\nok    render    12ms\n' > results.txt`
Enter
Type@0ms "clear"
Enter
Show
Sleep 400ms
Type "cat results.txt"
Enter
Wait
Sleep 900ms
# foley: highlight /FAIL.*/
Sleep 1800ms
# foley: highlight off
Sleep 500ms
