# One tape, two assets: this tape sets NO theme, so -theme decides the
# palette at record time — the dark/light pair GitHub's
# <picture prefers-color-scheme> wants, from a single source:
#
#   foley -theme "Catppuccin Mocha" -o dark.gif  demo.tape
#   foley -theme "Catppuccin Latte" -o light.gif demo.tape

Output pair.gif
# foley: dress macos

Set Shell bash
Set Width 860
Set Height 380
Set FontSize 15
Set TypingSpeed 35ms

Hide
Type@0ms "cd $(mktemp -d) && cat > release.sh <<'SH'"
Enter
Type@0ms "printf '\033[33m→\033[0m building 3 targets\n'"
Enter
Type@0ms "printf '\033[32m✓\033[0m darwin-arm64   4.1s\n'"
Enter
Type@0ms "printf '\033[32m✓\033[0m linux-amd64    3.8s\n'"
Enter
Type@0ms "printf '\033[32m✓\033[0m linux-arm64    3.9s\n'"
Enter
Type@0ms "printf '\033[36mrelease v1.4.0 ready\033[0m\n'"
Enter
Type@0ms "SH"
Enter
Type@0ms "printf 'release: ; @sh release.sh\n' > Makefile && clear"
Enter
Show

Type "make release"
Enter
Sleep 2500ms
