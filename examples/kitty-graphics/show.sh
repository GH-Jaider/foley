#!/bin/sh
# Transmits a small PNG through the kitty graphics protocol (APC G,
# a=T f=100: transmit-and-display PNG at the cursor) and labels it.
# Self-contained: the image travels base64-embedded in this script.
set -eu

printf 'kitty graphics over a pty — no window anywhere\n'
# shellcheck disable=SC1003 # the trailing \\ is printf for backslash: ESC \ is the APC terminator (ST), not a quote escape
printf '\033_Gf=100,a=T;iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAIAAABvFaqvAAAAOklEQVR4nGL5yrCUgRDgvh5FUA0TQRVEglGDRg0angYx/n9IWJHm/zyCagaf10YNGjVoUBkECAAA//+M+AZ+7qTArAAAAABJRU5ErkJggg==\033\\'
printf '\n\ndone\n'
