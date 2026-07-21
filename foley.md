---
name: foley
description: Author and record terminal demos with foley — VHS-style .tape scripts plus "# foley:" post-production cues (dress, keys, highlight, zoom, studio), rendered without a terminal window. Use when creating, editing, debugging or recording .tape files, or whenever a demo gif/mp4/asciicast/golden-text of a CLI or TUI is needed.
---

# foley — the agent's manual

foley renders terminal demos from `.tape` scripts. There is no window
and no screen capture: the demo command runs on a real pty, an
embedded terminal engine (libghostty-vt) keeps the screen, and foley
draws every frame itself. Consequences you can rely on:

- **Deterministic by default**: the same tape produces byte-identical
  output on any machine. Demos double as regression tests.
- **kitty graphics render natively** (TERM is `xterm-ghostty`, real
  terminfo shipped): file managers with image previews, pixel-art
  players, `mpv --vo=kitty` — all recordable.
- **Post-production is free**: camera zooms, highlights and a
  keystroke reel are cues in comments, applied at render time. The
  footage (the pty session) is never touched.
- Headless: works in CI, no permissions, no display server.

Recording: `foley demo.tape`. Everything below is the complete
authoring reference; `foley manual` prints the same grammar from the
binary itself and `foley -h` the flags — trust those over memory.

## The agent loop

Never fire blind. The loop that works:

1. **Write** the tape (or `foley new draft.tape` for a scaffold).
2. **`foley validate demo.tape`** — parses grammar AND cues, prints
   the cue sheet, records nothing. A cue typo is a loud parse error
   here, never a silent no-op at record time.
3. **Record** with inspection outputs alongside the real one — extra
   `Output` lines or `-o` flags are cheap:
   - `Output demo.gif` — the deliverable.
   - `Output demo.txt` — **the final screen as plain text.** Read it
     to verify content, and count rows/columns in it to aim cues —
     no pixel math. (kitty-graphics pixels don't appear in text.)
   - `Screenshot beat.png` — a frame exactly where you put the
     command in the script. Drop one at every beat you must verify.
   - `Output frames/` — every frame as PNG plus a timing manifest,
     when you need the whole timeline.
4. **Inspect, then iterate.** When sampling a finished **gif**,
   sample by *time*, never by frame index — gif frames coalesce
   (a 3-second still is one long frame): `ffmpeg -i demo.gif -vf
   fps=2 t%03d.png`.

## Tape grammar

One command per line. Strings quote with `"…"`, `'…'` or `` `…` ``
(backticks let you nest quotes freely). `#` starts a comment.
Relative paths (Output, Screenshot, Source, files your commands
read) resolve against the working directory foley runs in.

```
Output <path>              # deliverable; repeatable; format by extension
Require <program>          # fail before recording if missing from PATH
Set <setting> <value>      # see Settings
Env <key> "<value>"        # environment for the recorded shell (value is a STRING — quote it)
Type[@<time>] "<string>"   # type it; @time = per-keystroke delay (Type@0ms pastes instantly)
Sleep <time>               # 500ms, 2s — realtime: wall clock; deterministic: virtual
Wait[+Screen|+Line][@<timeout>] [/<regexp>/]   # block until match; bare Wait = shell prompt
Ctrl[+Alt][+Shift]+<char>  # chords: Ctrl+C, Ctrl+Shift+P, Alt+Enter, Shift+Tab
Enter/Backspace/Delete/Insert/Tab/Space/Escape [n]
Up/Down/Left/Right [n]     # arrows — modifiers do NOT combine with them
PageUp/PageDown [n]        # (no Home/End in the grammar)
ScrollUp/ScrollDown [n]    # terminal viewport scroll
Hide / Show                # stop/resume emitting frames (the pty keeps running)
Screenshot <path>.png      # one frame, here
Copy "<string>" / Paste    # internal clipboard
Source <path>.tape         # splice another tape (cues are NOT allowed inside it)
```

Command notes that save takes:

- **`Type@0ms` is for scaffolding**, not for driving TUIs: a running
  application can miss an instantaneous keypress. Send keys a TUI
  must react to at natural speed (`Type "q"`, default TypingSpeed).
- **Bare `Wait` matches the shell prompt** — and if you `Env PS1
  "MINE ❯ "`, it learns yours automatically. After any command of
  unknown duration (an animation, a build), `Wait` beats guessing
  with Sleep. Cap patience per-wait (`Wait@30s`) or globally
  (`Set WaitTimeout 30s`).
- **Hide/Show scaffolding pattern**: do setup off camera and end the
  hidden block with `clear`, so the first visible frame is clean:

  ```
  Hide
  Type@0ms "./stage-props.sh && clear"
  Enter
  Show
  ```
- **Keep typed strings ASCII.** Emoji or heavy unicode inside `Type`
  is fragile end-to-end; put fancy text in a sourced script or a
  file the recorded shell reads, and type only its plain name.
- `Env` values are strings — `Env TENTEN_PIXEL "1"`, never a bare 1.

## Settings

```
Set Shell <string>            # bash recommended; the prompt is pinned and Wait knows it
Set Width/Height <px>         # window size (logical px) → derives the cell grid
Set FontSize <n>              # with Width/Height decides columns × rows
Set FontFamily <name>         # pinned catalog only (foley fonts) — never system fonts
Set TypingSpeed <time>        # default per-keystroke delay (override per-Type with @)
Set Theme <name|{json}>       # curated list: foley themes
Set Padding/Margin <px>  ·  Set MarginFill <#hex>
Set WindowBar <type>          # Colorful|ColorfulRight|Rings|RingsRight|LinuxControls|GnomeCSD
Set WindowBarSize/BorderRadius <px>
Set Framerate <n>             # realtime capture rate (deterministic emits exact frames)
Set PlaybackSpeed <float>     # scale final timing
Set WaitTimeout <time>  ·  Set WaitPattern <regexp>
```

Parsed but **staged** (accepted with a loud warning, no visual
effect yet — don't spend takes tuning them): `LetterSpacing`,
`LineHeight`, `CursorBlink` (renders solid), `LoopOffset`.
Compatibility gaps are never silent: if a setting is staged or
diverges from VHS, recording says so on stderr.

Prefer a `dress` cue over hand-setting chrome (theme/bar/margins) —
explicit `Set`s always win over the dress, so targeted overrides
stay possible.

## The cues — post-production

Written as comments, so the tape still runs under plain VHS. One cue
per line, on its own line. Inside the `# foley:` namespace parsing is
strict: `foley validate` rejects typos. `dress`, `keys` and `studio`
shape the whole take; `highlight` and `zoom` act at their position in
the script — between the commands around them, with the neighboring
`Sleep`s as the hold.

```
# foley: studio
# foley: dress <name | ./file.json | {json} | none>
# foley: keys [small|medium|large] [notation=keycap|icons] [accent=<ansi|#hex|off>] [plain]
# foley: highlight /<regexp>/ [<n>] [as <name>]
# foley: highlight <col>,<row> <w>x<h> [as <name>]
# foley: highlight off [<name>]
# foley: zoom <col>,<row> <w>x<h> [<duration>]
# foley: zoom off [<duration>]
```

### studio — a closed set

Fresh HOME, working directory and temp/XDG defaults for the take,
struck afterwards; the env identity is `foley@studio`. Nothing of
the real machine lands on camera, and the take leaves nothing
behind. It is set hygiene, not sandboxing: the set moves the
**defaults**, it forbids nothing. Practicalities:

- The boundary: an app handed an absolute host path still reads it
  (that is exactly how `-env PROPS=` below gets files in), and
  kernel-level identity (hostname, `$HOST`) still shows the host.
  For a hard boundary, record in the container.
- Files must be **brought onto the set**: pass locations via
  `-env PROPS=/abs/path` and copy/read from `"$PROPS"` in a hidden
  setup block.
- Apps see a virgin config. If one asks a first-run question you
  don't want on camera, **plant its config file** during setup —
  XDG paths point inside the set, e.g.:

  ```
  Type@0ms `CFG="${XDG_CONFIG_HOME:-$HOME/.config}/tenten" && mkdir -p "$CFG" && printf '{"pixel":true,"sound":false}\n' > "$CFG/config.json" && clear`
  ```

### dress — the wardrobe

Theme, font, bar, padding, margins as one named layer. Built-ins:
`foley wardrobe`; scaffold your own: `foley sew`. Precedence:
defaults < dress < the tape's explicit `Set`s.

### keys — the input reel

A film strip under the window showing every keystroke with exact
timing (foley is the one typing, so the track is emitted, not
guessed). `small|medium|large`, `notation=keycap|icons`,
`accent=<ansi|#hex|off>`, `plain` (no strip dressing). Hidden
segments don't land on the reel.

### highlight — point the eye

A band of the theme's Selection color, from that beat until `off`.
The `/regexp/` form matches **screen text** and re-matches every
frame (`<n>` picks the n-th match, 0-based, screen order). Text
drawn as block-glyph art (figlet/cfonts output) is *not* text — use
the rect form: `<col>,<row> <w>x<h>`, cells, 0-based. Name
concurrent highlights (`as err`) to switch them off individually
(`highlight off err`).

### zoom — the camera

Push onto a cell rect, hold, pull back. The rect **fills the
frame**; duration is the move itself (default 600ms, cap 10s, no
easing — the duration is the shot). Always a 1:1 crop of the 2×
supersampled master, never an upscale — so there is a sharpness
floor: past 2× magnification foley refuses the take and the error
names the minimum rect (about 53×13 on a 940×520 @ FontSize 15
grid). A longer look = a longer `Sleep` while framed, not a longer
duration.

**Aiming a rect** (zoom or highlight): record once with `Output
probe.txt` and count rows/columns in the text — exact, no pixels.
For graphics regions (kitty pixels are invisible in .txt), take a
`Screenshot`, find a character whose column you know (the prompt's
first char is col 0), derive px-per-cell from two known characters,
then convert. Re-measure if you change Width/Height/FontSize — and
beware: TUIs that size themselves from the pty's *pixel* report
(kitty-graphics apps) can change their own footprint when
`-output-scale` changes. Aim at the scale you ship.

## Two clocks

- **Deterministic** (default): virtual time; output attributed to
  the step that caused it; byte-identical reruns; renders faster
  than real time. The right clock for shells, CLI runs, waits. The
  byte-identity covers what foley controls — an app that ticks
  (status clocks, random IDs) changes its own footage; freeze it
  before goldening a take. Caveat: the virtual clock advances on
  pty *silence*, and an app's own async startup (spawning servers,
  loading config) is silence too — a `Sleep` after launching a TUI
  stamps time, it does not wait, so the take can move on before the
  interface draws. Symptom: a full-screen app records blank. Fix:
  anchor on something it draws — `Wait+Screen /text/` — which
  synchronizes with the real app and keeps the take deterministic.
  (Terminal queries at startup — colors, pixel size, capabilities —
  are answered instantly; they are not the stall.)
- **Realtime** (`-mode realtime`): wall clock, captures every byte
  as it happened. The only clock for continuously-animating content
  (spinners you want honest, `tte`/`cmatrix`-style animations,
  pixel-art players, video in the terminal). Sleeps spend real
  seconds; animations of unknown length end with `Wait`. Full-screen
  animation through a pty is I/O-bound — pacing changes with content
  size, so time the take from the recording, not from arithmetic.

## Outputs

Format follows the extension, from `Output` lines or `-o` (which
replaces them). Multiple outputs per take are fine.

| Extension | You get |
|---|---|
| `.gif` / `.mp4` / `.webm` / `.webp` | video, reproducibly encoded |
| `.cast` | asciicast v2 for asciinema players |
| `.txt` | final screen as text — verification and CI goldens |
| `.png` (via `Screenshot`) | single frames at script positions |
| a directory | every frame as PNG + timing manifest |

`-output-scale 2` (default) is retina; `1` is logical size (~¼ the
bytes, hairlines soften). `-gif-loop 0` loops forever.

## CLI

```
foley [flags] <file.tape | ->        record (- = stdin)
foley play demo.tape                 record, then screen it in this terminal (kitty graphics)
foley validate demo.tape             lint + cue sheet, nothing records
foley new demo.tape  ·  foley sew <name>     scaffolds (tape / dress)
foley themes · fonts · wardrobe      the catalogs
foley doctor                         check fonts, engine, ffmpeg
foley manual                         this grammar, from the binary
```

Key flags (all can go before or after the tape path):

```
-mode deterministic|realtime   the clock (see Two clocks)
-studio                        closed set without editing the tape
-env KEY=VALUE                 add env (repeatable; wins over the tape's Env)
-dir <path>                    cwd for the recorded command only
-o <path>                      output override (repeatable, replaces tape Outputs)
-output-scale 2|1              retina | logical
-dress <name|file|{json}|none> replace the dress layer
-keys <tokens|off>             replace the keys layer
-theme <name|{json}>           replace the palette — record dark/light pairs from one tape
-watch                         re-record on every save of tape/Source/dress
-cols/-rows <n>                force the grid (else derived from Width/Height/FontSize)
-gif-loop 0|-1|N               loop forever | once | N extra
```

## Recipes

**Custom prompt** (and Wait still works):

```
Env PS1 "ACTION ❯ "
Type "make test"
Enter
Wait
```

**Dark/light pair from one theme-less tape** — then let GitHub pick:

```
foley -theme "Catppuccin Mocha" -o dark.gif  demo.tape
foley -theme "Catppuccin Latte" -o light.gif demo.tape
```

**Demos as tests**: add `Output golden.txt`, commit it, diff on CI.
Deterministic mode guarantees byte-identical reruns.

**kitty graphics**: any app that detects support by TERM sees a
kitty-capable terminal. To place an image yourself:

```
printf '\033_Gf=100,a=T,c=12,r=5;%s\033\\' "$(base64 < img.png | tr -d '\n')"
```

(`f=100` PNG, `a=T` transmit+display, `c=,r=` stretch into that
cell rect at the cursor.)

**Common failures** — what the error means:

| Symptom | Cause → fix |
|---|---|
| `zoom: N× exceeds the 2× sharp limit` | rect too small — widen to the minimum the error names |
| `Env X expects string` | quote the value: `Env X "1"` |
| `unknown cue` / cue parse error | typo in the `# foley:` namespace — validate catches it before recording |
| `` `# foley:` cues must be on their own line `` | a cue after a command on the same line — move it to its own line |
| `Source'd tape carries cues` | cues only live in the top-level tape |
| take hangs then fails at a Wait | raise `Set WaitTimeout` (animations, slow builds) or fix the pattern |
| a TUI ignored a key | it was sent too fast — drop `@0ms`, use natural speed |
| `Alt+Tab`/modified named key reached the app as plain | legacy encoding, as in VHS — record with `--modify-other-keys` for CSI-27 forms |

## Ground truth

When this file and the binary disagree, the binary wins: `foley
manual` (grammar and cues), `foley -h` (flags), `foley validate`
(the judge). The full VHS command reference (same grammar, pinned):
https://github.com/charmbracelet/vhs#vhs-command-reference — and the
worked examples live in `examples/` in this repository, each with
the tape that recorded it.
