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
internal clipboard) warn loudly at run time — see ADR-008's three-tier
contract in the repository docs.

## What VHS cannot record

The examples in this directory showcase foley-only ground:

- **`kitty-graphics/`** — a demo that transmits a real image through the
  kitty graphics protocol. VHS's xterm.js cannot display it at all;
  foley's embedded engine decodes the transmission and composites the
  image into the recording, byte-exactly, on any machine.
