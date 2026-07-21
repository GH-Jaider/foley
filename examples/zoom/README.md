# zoom — drive the camera

`# foley: zoom` eases the camera onto a **cell rect** and stays framed
there until the next `zoom` or `zoom off`. Post-production over a
sharper negative: with a zoom cue anywhere in the tape, the scene
renders on a 2× supersampled master and every output frame is an exact
integer downscale of it — a full 2× zoom is a 1:1 crop of the master.
No upscaling ever happens, so zoomed frames are as sharp as unzoomed
ones. VHS ignores the cue and still records, unzoomed.

```tape
Type "make test"
Enter
Sleep 800ms
# foley: zoom 0,1 36x9 700ms   ← ease onto rows 1–9...
Sleep 1500ms
# foley: zoom off 700ms        ← ...and back out
```

## The two forms

| Form | Example | What it does |
|---|---|---|
| rect | `# foley: zoom 0,1 36x9` | frame those cells (COL,ROW WxH, 0-based — one standard with highlight rects) |
| off | `# foley: zoom off` | ease back to the full frame |

Both take an optional trailing duration (`700ms`, `1s`) — the
transition time. Absent means 600ms; the cap is 10s (each second of
transition renders ~30 physical frames — a slower reveal is a longer
`Sleep` while framed, not a longer transition). `Set PlaybackSpeed`
scales transitions like everything else on the timeline. There is no
easing knob: **the duration is the shot**. The camera never cuts — a
new zoom issued mid-transition departs from wherever the lens is at
that instant, and a move to where it already rests (an `off` at rest,
re-zooming the same rect) emits nothing at all.

## Aiming the rect

Cells are 0-based: `0,0` is the window's top-left character. Don't
count pixels to find your rect — probe the take as text:

1. Record once with a text output: `foley -o probe.txt demo.tape`
   (replaces the tape's `Output` lines — the `.txt` is the final
   screen, character-exact, and no gif gets encoded).
2. Open `probe.txt` and count: the line number your target sits on is
   its **ROW**, the character offset where it starts is the **COL**,
   and the characters it spans are the **W** (all 0-based).
3. Write the rect from that — e.g. `# foley: zoom 40,0 56x13 700ms`.

Two caveats: the `.txt` is the *final* screen, so if your moment
scrolls away before the end, park the probe take there with a long
`Sleep`; and re-measure whenever `Set Width` / `Height` / `FontSize`
change — the grid moves with them. Aim slightly tight around the thing
you want: the camera grows the rect to the output's aspect on its own
(next section).

## What the rect becomes

The rect is a *region of interest*, not the literal frame: it grows to
the output's aspect ratio around its center and clamps inside the
window, so the result is always a clean crop with no letterboxing. The
keys reel (if on) stays pinned under the window at full size — it's a
HUD on the camera glass, not part of the scene.

## The 2× limit

The camera refuses to zoom past 2× — beyond the master's supersample it
would be inventing pixels, and foley never ships a blurry frame. A rect
too small to frame sharply fails **before recording starts** (`foley
validate` can't check it — the limit needs the real font geometry — but
the run pre-flights every cue at frame zero, never mid-take). The error
tells you the minimum rect size for your canvas.

Recording with the camera costs ~4× render time and memory (the master
is 2× in each dimension). Tapes without zoom cues never pay it — their
output is byte-identical with the feature merely existing.

Library API: `Recorder.Zoom(col, row, w, h, dur)` /
`Recorder.ZoomOff(dur)` — enable with `Options.Zoom: true`.
