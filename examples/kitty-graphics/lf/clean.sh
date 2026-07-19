#!/bin/sh
# lf cleaner: drop every visible placement before the next preview —
# a=d is the kitty protocol's delete, the counterpart of preview.sh's
# transmit.
set -eu
# shellcheck disable=SC1003 # the trailing \\ is printf for backslash: ESC \ is the APC terminator (ST), not a quote escape
printf '\033_Ga=d,d=A\033\\' >/dev/tty
