# foley cross-OS reference tape (M9, PRD metric 1): the SAME tape must
# produce byte-identical frames on macOS and Linux. Deterministic mode +
# frames output (pure PNG, no ffmpeg), and every command below is
# portable across bash 3.2 (macOS) and bash 5 (Linux) by construction:
# printf with octal escapes only — no locale, no \u, no echo -e.
#
# It exercises the full render surface: SGR (bold/italic/inverse/256/
# truecolor/underline), synthesized sprites (box drawing incl. doubles,
# heavy, rounded, dashes, diagonals; blocks; braille), text shaping, and
# a kitty-graphics PNG placement (f=100, base64, scaled to a cell rect).
#
# CI runs it twice per OS (within-OS stability) and compares the frame
# manifest across OSes. Run locally:
#   foley -fonts internal/fontpack/fonts ci/crossos.tape

Output frames/

Set Shell "bash"
Set FontSize 16
Set Width 720
Set Height 400
Set Padding 12
Set TypingSpeed 0ms

Hide
Type "clear"
Enter
Sleep 200ms
Show

# Sprite panel: rounded box, blocks, braille, heavy/double junctions,
# diagonals, dashes, quadrants, eighths — octal UTF-8, bulletproof.
Type `printf '\033[1;32m\342\225\255\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\225\256\033[0m \033[38;5;213m\342\226\200\342\226\204\342\226\210\342\226\214\342\226\220\342\226\221\342\226\222\342\226\223\033[0m \033[3;36m\342\240\233\342\243\277\033[0m\n'`
Enter
Sleep 300ms
Type `printf '\033[1;32m\342\224\202\033[0m foley  \033[7mQA\033[0m \033[1;32m\342\224\202\033[0m \033[38;2;217;119;87m\342\224\217\342\224\201\342\225\213\342\224\201\342\224\223 \342\225\261\342\225\262 \342\224\206\342\224\204\033[0m\n'`
Enter
Sleep 300ms
Type `printf '\033[1;32m\342\225\260\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\224\200\342\225\257\033[0m \033[4munder\033[0m \342\225\220\342\225\254\342\225\220 \342\226\226\342\226\232\342\226\236\342\226\227 \342\226\201\342\226\203\342\226\205\342\226\207\n'`
Enter
Sleep 500ms

# Kitty graphics: 4x4 quadrant PNG stretched over a 6x3 cell rect.
Type `printf '\033_Ga=T,f=100,c=6,r=3;iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAIAAAAmkwkpAAAAHklEQVR42mP4yrAUiBiu/wchFM7/hyCk8SAPiFA4AMhaGxmZQ5/RAAAAAElFTkSuQmCC\033\\'`
Enter
Sleep 1s
