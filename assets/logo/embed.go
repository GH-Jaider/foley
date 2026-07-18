// Package logo embeds the brand assets the CLI shows at its front
// door — the installed binary stays self-contained wherever it runs.
// The files are produced by `make logo` (the logo IS a recording:
// see logo.tape and tooling/logogen).
package logo

import (
	_ "embed"
	"fmt"
	"strings"
)

// GIF is the projector: the film chip typing the name letter by
// letter, REC light blinking, looping.
//
//go:embed logo.gif
var GIF []byte

// CellArt is the film chip in terminal cells: half-blocks for the
// edges, truecolor for film/screen/ink/REC, and the sprocket holes
// painted with NO color at all — the terminal's background shows
// through, exactly like the punched film.
func CellArt() string {
	const w = 33
	bg := func(r, g, b int) string { return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b) }
	fg := func(r, g, b int) string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b) }
	const reset = "\x1b[0m"
	film := bg(0x19, 0x15, 0x14)
	filmFg := fg(0x19, 0x15, 0x14)
	scr := bg(0x0D, 0x0B, 0x0A)
	ink := fg(0xEC, 0xE6, 0xDF)
	rec := bg(0xFF, 0x4F, 0x45)

	sp := strings.Repeat
	holes := film + sp(" ", 3)
	for i := 0; i < 6; i++ {
		holes += reset + sp(" ", 2)
		if i < 5 {
			holes += film + sp(" ", 3)
		}
	}
	holes = holes + film + sp(" ", 3) + reset

	var b strings.Builder
	line := func(s string) { b.WriteString("  " + s + "\n") }
	line(filmFg + sp("▄", w) + reset)
	line(holes)
	line(film + sp(" ", w) + reset)
	line(film + sp(" ", 2) + scr + sp(" ", w-4) + film + sp(" ", 2) + reset)
	line(film + sp(" ", 2) + scr + sp(" ", 3) + ink + "\x1b[1m>foley\x1b[22m" + rec + " " + scr + sp(" ", w-4-3-6-1) + film + sp(" ", 2) + reset)
	line(film + sp(" ", 2) + scr + sp(" ", w-4) + film + sp(" ", 2) + reset)
	line(film + sp(" ", w) + reset)
	line(holes)
	line(filmFg + sp("▀", w) + reset)
	return b.String() + "\n"
}
