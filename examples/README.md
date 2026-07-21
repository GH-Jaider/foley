# foley examples

**Every VHS tape is a foley tape.** foley vendors VHS's real grammar
(pinned release, see `scripts/vendor-vhs.sh`) and its entire upstream
examples corpus — all 106 example tapes from
[charmbracelet/vhs](https://github.com/charmbracelet/vhs) live in
`tape/internal/vhsgrammar/examples/` and are conformance-tested on every
run: they parse identically here, and the terminal semantics (typing,
chords, waits against the real prompt) execute faithfully. Grab any tape
you already have and run it:

```sh
foley demo.tape
```

Compatibility gaps are never silent: staged visuals (window bar,
margins, cursor blink) and deliberate divergences (pinned fonts,
internal clipboard) warn loudly at run time — parsed always, executed
faithfully or warned, never silent.

## What VHS cannot record

The examples in this directory showcase foley-only ground. Most run
inside the **studio** (`# foley: studio`): a closed set per take —
fresh HOME, working directory and temp defaults, struck when the
recording ends — so nothing of the author's machine lands on camera
(it moves the defaults; it forbids nothing — absolute host paths
still work). Regenerate everything with `make examples`.

- **`fetch/`** — the hero shot: real fastfetch, the film-chip logo,
  and a `Terminal: foley` line that is GENUINE — foley IS the
  terminal the demo runs in.
- **`keys/`** — the input reel (`# foley: keys`) on its motivating
  case: lazygit. j/k to move, an invisible spacebar to stage, `c` +
  a message to commit — every keystroke lands as a frame on the film
  strip under the window, with exact timing: foley is the one typing,
  so the track is emitted, not captured. The most-upvoted VHS
  request, structural here.
- **`zoom/`** — the camera (`# foley: zoom`) earning its keep: tmux
  splits the screen (the `^B %` chord narrated by the reel), the
  right pane runs the tests, and the camera pushes onto the FAIL
  until it reads — a 1:1 crop from the supersampled master, never an
  upscale. The bar follows tmux's OSC title (`windowTitleFollow`).
- **`kitty-graphics/`** — the corner VHS cannot enter: its xterm.js
  cannot display the protocol at all. The raw demo (a PNG transmitted
  by a shell script, composited byte-exactly), plus two richer takes:
  **`lf/`** — image previews inside a file manager (committed
  previewer/cleaner pair) — and **`tenten/`** — pixel art LIVING in
  the terminal, recorded in realtime mode: the one clock a continuous
  animation needs.
- **`highlight/`** — point the viewer's eye (`# foley: highlight`):
  the theme's Selection color under a /regex/ from its position in
  the script until `off`. The zombie theme field, finally employed.
- **`showcase/`** — the trailer: a gif is a silent film and foley is
  the craft that adds the sound, so every sound effect is WRITTEN —
  intertitle cards in trailer voice ([ BWAAAAAH ], [ APPLAUSE
  INTENSIFIES ]). The brand chip wipes in, cfonts + tte raise the
  title, a dossier decrypts under the highlight, and the premiere is
  pixel art living in kitty graphics with the camera pushing onto live
  pixels. Cast: `tte`, `cfonts`, `gum`, `tenten` — plus a hidden crew
  (`props/cast.sh`) the pty never sees typed: the tape only ever keys
  plain ASCII scene names. Recorded in realtime — the one clock the
  animations need.
- **`prompt/`** — your prompt, your rules: `Env PS1` (always legal
  grammar) actually WINS here, and a bare `Wait` learns the new
  prompt automatically. In VHS the same tape records with its pinned
  prompt — degradable by construction.
- **`pair/`** — ONE theme-less tape recorded twice with `-theme`: the
  dark/light pair GitHub's `<picture>` wants, from a single source.
- **`dresses/`** — one scene recorded under the whole wardrobe: the
  house dress (`foley`), genre chrome, the noir/paper pair, and a
  brand kit in a file (`# foley: dress …` — VHS ignores the cue and
  still records).
