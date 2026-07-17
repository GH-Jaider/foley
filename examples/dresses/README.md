# dresses — one tape, seven looks

A **dress** is an appearance preset for everything about how the footage
is *painted* — palette (`theme`), typography (`fontSize`), window chrome
— and nothing about what happened (grid, shell, timing). It travels
*inside* the tape as a cue comment — `# foley: dress warp` — so the same
file produces the same look everywhere (your CI included), and VHS
simply ignores the line: these tapes still record there, just without
the wardrobe.

| Tape | Look |
|---|---|
| `macos.tape` | traffic lights + centered `~` title, rounded — the macOS genre |
| `gnome.tape` | CSD close button + centered title — the Linux/GNOME genre |
| `iterm.tape` | traffic lights + LEFT title, subtle radius |
| `kitty.tape` | thin bar, lights + centered `~`, tight padding |
| `warp.tape` | lights, radius 10, dark margin band (inspired — foley never fakes another app's input layout: the footage is the footage) |
| `bare.tape` | content only — padding 0 |
| `brand.tape` | the full kit: own palette + type size + titled bar, from `brand.dress.json` — a brand identity that ships with the repo |

Try them:

```sh
foley warp.tape                  # the look the tape declares
foley -dress kitty warp.tape     # same tape, different layer
foley wardrobe warp              # what a dress expands to
```

The tape's own explicit `Set` commands always beat the dress — a dress is
a base layer, not a lock. Bring your own: `# foley: dress ./brand.dress.json`
(ships with your repo) or an inline `# foley: dress {"padding": 12}`.
Rebranding every demo is then a one-file edit; light/dark variants of the
same tape are `foley demo.tape -dress ./light.json` and `-dress ./dark.json`
— the two assets a GitHub `<picture prefers-color-scheme>` needs.
