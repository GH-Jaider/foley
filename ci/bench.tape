# foley benchmark tape (M9, PRD metric 3): ~30 seconds of DECLARED time
# that must export in under 5 wall-clock seconds — deterministic mode's
# whole point is rendering faster than real time. VHS-default geometry
# (1200x600, font 22 → 2400x1200 frames at 2x) so the numbers are
# honest, and a gif Output so ffmpeg's palette pass is part of the bill.
# Portable printf-octal like crossos.tape.
#
#   time foley -fonts internal/fontpack/fonts ci/bench.tape

Output bench.gif

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 600
Set Padding 60
Set TypingSpeed 50ms

Type "echo foley bench"
Enter
Sleep 5000ms

Type@0ms `printf '\033[1;32m\342\225\255\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\225\256\033[0m \033[38;5;213m\342\226\200\342\226\204\342\226\210\342\226\214\342\226\220\342\226\221\342\226\222\342\226\223\033[0m \033[3;36m\342\240\233\342\243\277\033[0m\n'`
Enter
Sleep 3500ms

Type@0ms `printf '\033[1;32m\342\224\202\033[0m foley  \033[7mQA\033[0m \033[1;32m\342\224\202\033[0m \033[38;2;217;119;87m\342\224\217\342\224\201\342\225\213\342\224\201\342\224\223 \342\225\261\342\225\262 \342\224\206\342\224\204\033[0m\n'`
Enter
Sleep 3500ms

Type@0ms `printf '\033[1;32m\342\225\260\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\225\257\033[0m \033[4munder\033[0m \342\225\220\342\225\254\342\225\220 \342\226\226\342\226\232\342\226\236\342\226\227 \342\226\201\342\226\203\342\226\205\342\226\207\n'`
Enter
Sleep 3500ms

Type@0ms `printf '\033_Ga=T,f=100,c=6,r=3;iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAIAAAAmkwkpAAAAHklEQVR42mP4yrAUiBiu/wchFM7/hyCk8SAPiFA4AMhaGxmZQ5/RAAAAAElFTkSuQmCC\033\\'`
Enter
Sleep 3500ms

Type "seq 1 12"
Enter
Sleep 3500ms

Type@0ms `printf '\033[1mdone\033[0m \033[38;5;46mok\033[0m \033[9mstruck\033[0m\n'`
Enter
Sleep 3500ms

Type "clear"
Enter
Sleep 4000ms
