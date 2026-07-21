# highlight — point the viewer's eye

`# foley: highlight` paints the theme's **Selection** color under
chosen text, from its position in the script until `off`. Pure
post-production: the terminal never knows, and VHS ignores the cue —
the tape still records there, unhighlighted.

```tape
Type "make test"
Enter
Sleep 800ms
# foley: highlight /^error.*/     ← from HERE...
Sleep 2s
# foley: highlight off            ← ...to here
```

## The three forms

| Form | Example | What it paints |
|---|---|---|
| regex | `# foley: highlight /error/` | every match, on every row, every frame — it follows the text if the screen scrolls |
| cell rect | `# foley: highlight 0,3 20x1` | exactly those cells (COL,ROW WxH, 0-based) — surgical, ignores text |
| off | `# foley: highlight off` | clears every active highlight |

## Modifiers

- **Match index** (patterns only): `/error/ 0` paints only the first
  match of each frame, `/error/ 1` the second, and so on — **0-based
  in screen order** (top-to-bottom, left-to-right), the same standard
  as the rect's cells.
- **Names**: `... as err` names a highlight; `off err` turns off just
  that one while others stay lit. An `off` for a name never declared
  earlier is a parse error — `foley validate` catches the typo.

```tape
# foley: highlight /error/ 0 as primero
# foley: highlight 0,5 12x1 as caja
Sleep 2s
# foley: highlight off primero    # caja sigue encendido
```

Several highlights can be active at once; a bare `off` clears them all.

## Aiming a rect

The rect form uses the same 0-based grid as zoom: `0,0` is the
window's top-left character. To find the cells without counting
pixels, probe the take as text — `foley -o probe.txt demo.tape`, then
count in `probe.txt`: the line number your target sits on is its
**ROW**, the character offset where it starts is the **COL**, the span
is the **W**. The `.txt` is the *final* screen, so if your moment
scrolls away, probe with a long `Sleep` parked at that moment — and
re-measure whenever `Set Width` / `Height` / `FontSize` change. If you
find yourself re-measuring often, that's the sign the regex form fits
better: it re-matches every frame and follows the text.

## Matching exactly what you mean

The pattern runs against each row's text, so a loose regex can catch
more than you want — e.g. `/error.*/` also matches the `error` inside
your own echoed command. Discriminate with context:

- **Only the output line**: anchor it — `/^error.*/` (the echoed
  command starts with the prompt, not with `error`).
- **Only the echo**: give it context — `/printf.*error/`.
- **Case-insensitive**: standard Go syntax — `/(?i)warning/`.
- **When text can't discriminate**: use the cell rect. It doesn't
  follow scrolling — it's a fixed spotlight on the stage.

A match never bleeds into empty screen space: `.*` stops at the last
real glyph of the row, not at the window edge.

## Where the color comes from

The theme's own `selection` slot (every terminal theme ships one —
Catppuccin Mocha `#585b70`, Latte `#acb0be`). Change the theme (or the
dress) and the highlight follows. Inline palettes may set
`"selection"` explicitly.

Library API: `Recorder.Highlight(spec)` / `Recorder.ClearHighlights()`.
