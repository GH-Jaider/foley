# Foley

Record scripted, VHS-style demos of terminal apps on a **real terminal** — kitty, running off-camera — at full fidelity: kitty graphics protocol, kitty keyboard protocol, synchronized output, ligatures, emoji, 60 fps, mp4/webm/GIF.

> In film, a foley artist recreates sound off-camera, with real objects instead of synthesizing it.
> Foley records terminal demos off-screen, with a real terminal instead of simulating one.

Library first (Go), CLI second. Familiar VHS-style DSL (`Type`, `Sleep`, `Wait`, `Set`, `Output`). Linux and macOS as first-class platforms: headless compositor on Linux, off-screen window on macOS. kitty-only in v1; more backends later.

## Status

Pre-alpha — design stage.

## License

MIT
