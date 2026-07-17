# dresses — one tape, six looks

A **dress** is an appearance preset: window bar, rounded corners, margin,
padding. It travels *inside* the tape as a cue comment — `# foley: dress
warp` — so the same file produces the same look everywhere (your CI
included), and VHS simply ignores the line: these tapes still record
there, just without the wardrobe.

| Tape | Look |
|---|---|
| `macos.tape` | traffic lights + centered `~` title, rounded — the macOS genre |
| `gnome.tape` | CSD close button + centered title — the Linux/GNOME genre |
| `iterm.tape` | traffic lights + LEFT title, subtle radius |
| `kitty.tape` | thin bar, lights + centered `~`, tight padding |
| `warp.tape` | lights, radius 10, dark margin band (inspired — foley never fakes another app's input layout: the footage is the footage) |
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
