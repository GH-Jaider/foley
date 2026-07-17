# dresses — one tape, four looks

A **dress** is an appearance preset: window bar, rounded corners, margin,
padding. It travels *inside* the tape as a cue comment — `# foley: dress
warp` — so the same file produces the same look everywhere (your CI
included), and VHS simply ignores the line: these tapes still record
there, just without the wardrobe.

| Tape | Look |
|---|---|
| `warp.tape` | Colorful bar, radius 10, dark margin band |
| `iterm.tape` | Colorful bar, radius 6, no margin |
| `kitty.tape` | no decorations, tight padding |
| `bare.tape` | content only — padding 0 |

Try them:

```sh
foley warp.tape                  # the look the tape declares
foley -dress kitty warp.tape     # same tape, different layer
foley wardrobe warp              # what a dress expands to
```

The tape's own explicit `Set` commands always beat the dress — a dress is
a base layer, not a lock. Bring your own: `# foley: dress ./brand.dress.json`
(ships with your repo) or an inline `# foley: dress {"padding": 12}`.
