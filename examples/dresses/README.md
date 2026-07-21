# dresses — one tape, a whole wardrobe

A **dress** is an appearance preset for everything about how the footage
is *painted* — palette (`theme`), typography (`fontSize`), window chrome
— and nothing about what happened (grid, shell, timing). It travels
*inside* the tape as a cue comment — `# foley: dress macos` — so the same
file produces the same look everywhere (your CI included), and VHS
simply ignores the line: these tapes still record there, just without
the wardrobe.

## Dress vs theme

A **theme** is the palette and only the palette — the 16 ANSI colors
plus foreground/background/cursor/selection. It comes from `Set Theme
"Catppuccin Mocha"` in the tape or `-theme "Catppuccin Latte"` on the
command line (`foley themes` lists the curated catalog), and it changes
colors, nothing else.

A **dress** is the whole paint layer in one named preset: the theme AND
the typography (`fontSize`) AND the window chrome — bar style and size,
title, border radius, padding, margin and its fill. Peek inside
`brand.dress.json`: its first field is a `theme`. A dress *contains* a
theme the way a wardrobe contains the shirt.

Rule of thumb: reach for a **theme** when only the colors change — the
dark/light pair of one tape is `-theme` twice (it wins over everything,
even an explicit `Set Theme`, and it swaps just the palette of whatever
the tape wears, dress included). Reach for a **dress** when the look is
an identity that should travel with the tape or the repo — one cue
line, and every recording (CI included) wears the whole thing.

| Tape | Look |
|---|---|
| `foley.tape` | the house dress: film-black margin, warm screen, REC cursor — the brand as wardrobe |
| `macos.tape` | traffic lights + centered `~` title, rounded — the macOS genre |
| `gnome.tape` | CSD close button + centered title — the Linux/GNOME genre |
| `bare.tape` | content only — padding 0 |
| `noir.tape` | the dark half of the built-in PAIR: TokyoNight + neutral window |
| `paper.tape` | the light half: Catppuccin Latte, same set — different lighting |
| `brand.tape` | the full kit: own palette + type size + titled bar, from `brand.dress.json` — a brand identity that ships with the repo |

Try them:

```sh
foley macos.tape                 # the look the tape declares
foley -dress noir macos.tape     # same tape, different layer
foley wardrobe macos             # what a dress expands to
```

The tape's own explicit `Set` commands always beat the dress — a dress is
a base layer, not a lock. Bring your own with `foley sew my-brand` (writes
a starter `my-brand.dress.json`; `-from macos` copies a built-in), then
`# foley: dress ./my-brand.dress.json` — it ships with your repo, and
paths inside it resolve relative to the file: the kit travels together.
Rebranding every demo is a one-file edit; light/dark variants of the same
tape are `foley demo.tape -dress noir` and `-dress paper` (or your own
pair) — the two assets a GitHub `<picture prefers-color-scheme>` needs.
