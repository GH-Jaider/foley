# Foley

Render scripted, VHS-style demos of terminal apps — no terminal window, no screen capture. Your app runs on a real pty; an embedded terminal engine (libghostty-vt, the brain of Ghostty) keeps the state; Foley rasterizes every frame itself: kitty graphics protocol, real ligatures, color emoji, 60 fps, mp4/webm/GIF — byte-identical output on macOS, Linux and CI, with zero permissions.

> In film, a foley artist recreates sound in the studio, with real objects instead of set recordings.
> Foley recreates the terminal in the studio, with your real app instead of a screen recording.

Library first (Go), CLI second. Familiar VHS-style DSL (`Type`, `Sleep`, `Wait`, `Set`, `Output`). Deterministic mode renders faster than real time; realtime mode sees every byte your app emits — no frame is ever missed.

## Status

Pre-alpha — no releases yet. The core pipeline is proven end to end: a real process on a pty, the embedded engine, and the rasterizer produce byte-identical frames across macOS and Linux, arm64 and amd64, verified in CI on every push. Recording (clocks, waits) and encoding are in progress.

## License

MIT
