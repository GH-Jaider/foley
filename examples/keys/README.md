# keys — the input reel

`# foley: keys` draws a **film strip under the window** showing every
keystroke with exact timing. foley is the one pressing the keys, so the
track is *emitted* with the footage — not captured and guessed. Words
print as real keycaps, arrows are drawn, the spacebar is a blank cap,
chords land as separate caps one beat apart.

Why it exists: a TUI's grammar is invisible keystrokes — `j`/`k` to
move, space to stage, `c` to commit — and without the reel the
recording shows a UI that changes by magic. With it, the viewer reads
*what was pressed* along with what happened.

```tape
# foley: keys
# foley: keys small notation=icons accent=off
```

Knobs are space-separated tokens in the cue (on the CLI, `-keys`
takes them separated by commas or spaces):

| Knob | Values | Default |
|---|---|---|
| size | `small` · `medium` · `large` | `medium` |
| `notation=` | `keycap` (words + drawn arrows) · `icons` (symbols) | `keycap` |
| `accent=` | an ANSI name, a `#hex`, or `off` — colors special/chord caps | the theme's bright magenta |
| `plain` | no strip dressing (no sprockets) | off |

## Ground rules

- **The canvas grows by the band.** The reel extends the frame below
  the window — footage is never covered and the grid never shrinks.
- **Hidden segments don't land.** What you type between `Hide` and
  `Show` (setup, secrets) never appears on the reel.
- **The CLI can overrule the tape.** `-keys` replaces the tape's own
  cue entirely: `foley -keys off demo.tape` strips the reel,
  `foley -keys large,notation=icons demo.tape` restyles it — and a tape
  with no cue at all gains one with `-keys on`.
- **Degradable by construction.** VHS ignores the cue line and still
  records the tape — silently, like every VHS gif of a TUI.

This demo drives lazygit (`Require lazygit` + `git`): `j`/`k` walk the
files panel, an invisible spacebar stages, `c` + a message commits —
watch the strip narrate each chord as the UI reacts.

Library API: `Options.KeysOverlay` (+ `KeysSize` / `KeysNotation` /
`KeysAccent`).
