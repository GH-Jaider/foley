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

- **`ScrollUp` / `ScrollDown` record for real** — the last staged VHS
  commands come alive. The viewport scrolls through the scrollback
  exactly like VHS's `term.scrollLines`: a view change the recorded
  application never sees, clamped at both ends, a no-op on the
  alternate screen (a TUI scrolls with its own keys). `ScrollUp@50ms 5`
  animates line by line; without a speed the scrolled view lands on
  the next hold. The library grows `Recorder.Scroll(delta)`, the
  engine contract `ScrollViewport`, and the conformance suite pins the
  semantics.
- **`foley validate` signs off out loud.** A clean session used to end
  in silence — indistinguishable from "did nothing" for humans and
  agents alike. Every validated tape now closes with one line:
  `demo.tape: ok — commands: 14 · cues: 2 · outputs: demo.gif`.

### Fixed

- **The manual no longer teaches phantom keys.** `foley manual` listed
  `Home`/`End`, but neither is a command in the pinned VHS grammar:
  `End` is a parse error, and `Home` — worse — lexes as a bare string
  and glues itself into the preceding `Type` as literal text. Both
  rows are gone.

### Changed

- **`make engine-lib` builds only your platform's engine.** It used to
  build all four release targets — a from-source build on linux spent
  minutes compiling macOS libraries it would never link. The full set
  moved to `make engine-lib-all`; CI and the release workflow always
  named their targets explicitly and are unaffected.

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
- **A terminal that answers back**: the startup interrogation a modern
  TUI fires gets immediate answers — XTWINOPS geometry reports
  (`11t/13t/14t/15t/16t/18t/19t`), XTGETTCAP served verbatim from the
  pinned `xterm-ghostty` terminfo (`TN`, `colors`, `Tc`, `Su`, `Smulx`,
  `Ms`, `setrgbf`, `setrgbb` — neovim detects truecolor and curly
  underlines on camera), DECRQSS, and the `CSI ?996n` color-scheme
  report, answered dark/light from the live background. Unknown
  capabilities get an instant negative — a prompt "no" ends a reply
  timeout as well as a "yes" — so opencode-class TUIs record correctly
  in deterministic mode, anchored on a `Wait+Screen /text/`.
- **kitty graphics**, rendered natively: image previews, pixel-art
  players, anything that speaks the protocol. Placements scroll with
  their content and clip at the terminal's content edge, like any real
  terminal's.
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
