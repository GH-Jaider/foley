# Changelog

All notable changes to foley are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
release workflow lifts each tag's section verbatim into its GitHub
release notes.

## Versioning — the v0.x policy

foley is pre-1.0. While the major version is `0`, the tape grammar and
the `# foley:` cues are the stable contract — a tape that records today
keeps recording — but Go API surface, flags and output framing may
still shift between minor versions. Breaking changes are called out
under **Changed** with a migration note. 1.0 is when the library API
freezes too.

Releases are cut from a `vX.Y.Z` tag; every 0.x release is marked a
pre-release on GitHub.

## Unreleased

### Added

- **The engine answers the full startup interrogation of modern
  TUIs.** XTWINOPS geometry reports (`11t/13t/14t/15t/16t/18t/19t` —
  pixel geometry from the same source as the pty winsize), XTGETTCAP
  serving the pinned `xterm-ghostty` terminfo story verbatim (`TN`,
  `colors`, `Tc`, `Su`, `Smulx`, `Ms`, `setrgbf`, `setrgbb` — so
  neovim detects truecolor and curly underlines on camera; a drift
  test pins every served value to the terminfo source), DECRQSS
  (vim's startup probes, immediate xterm-convention negative) and the
  `CSI ?996n` color-scheme report, answered dark/light from the LIVE
  effective background — an app's own OSC 11 flips it. Unknown
  capabilities get an instant negative: a prompt "no" ends a reply
  timeout as well as a "yes". Apps like opencode no longer burn their
  startup waiting out timeouts; paired with a `Wait+Screen /text/`
  anchor they record correctly in deterministic mode.

### Fixed

- **kitty placements now scroll with their content.** The engine's
  terminal ran with zero scrollback, so when an image's anchor line
  scrolled off-screen the placement was re-pinned to the top of the
  viewport and parked there — painting over unrelated text (visible
  as the `foley` welcome logo covering its own help text in short
  grids). Placements now scroll out like in any real terminal.
- **Placements clip to the grid.** A partially scrolled-out image no
  longer bleeds over the window padding/chrome; it is clipped at the
  terminal content edge, as a real terminal clips it.

## v0.1.0

The first public release: record VHS-style `.tape` scripts to
gif/mp4/webm/webp/asciicast/text without a terminal window, plus the
`# foley:` post-production cues.

### Added

- **The renderer**: your app runs on a real pty, libghostty-vt keeps
  the screen, foley draws every frame. Deterministic by default —
  byte-identical output on any machine, so demos double as regression
  tests — with a realtime clock (`-mode realtime`) for continuously
  animating content.
- **kitty graphics**, rendered natively: image previews, pixel-art
  players, anything that speaks the protocol.
- **Post-production cues** (`# foley:` comments, ignored by plain VHS):
  `dress` (window wardrobe), `keys` (the input reel), `highlight`,
  `zoom` (the camera), `studio` (a closed set — your machine stays off
  camera).
- **Outputs** by extension: `.gif` `.mp4` `.webm` `.webp` `.cast`
  `.txt`, `Screenshot` PNGs, or a frame directory with a timing
  manifest.
- **CLI**: `foley`, `play`, `validate`, `new`, `sew`, `doctor`,
  `themes`, `fonts`, `wardrobe`, `manual`, `completion` — flags before
  or after the tape path.
- **`foley skill`**: the whole grammar, cues, CLI and authoring loop as
  one loadable file ([`foley.md`](foley.md)) for AI agents that write
  tapes.
- **Self-contained binaries**: the pinned fonts and terminal engine are
  baked in, so a downloaded binary needs no `$FOLEY_FONTS` — one file,
  it just runs. `ffmpeg` is the only runtime dependency.
- **Distribution**: prebuilt tarballs for darwin/linux · arm64/amd64
  with checksums, a Homebrew tap (`brew install GH-Jaider/foley/foley`),
  and a 72 MB container image.

### Known limitations

- `LetterSpacing`, `LineHeight`, `CursorBlink` and `LoopOffset` parse
  (VHS compatibility) but are staged — accepted with a loud warning, no
  visual effect yet.
- `go install` needs the cgo toolchain and the pinned engine `.a`; the
  prebuilt binaries and `brew` are the frictionless paths.
