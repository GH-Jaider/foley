package raster_test

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
)

// spriteRig renders single-frame sprite scenes against the fake engine,
// on tenten-like fixture colors (fg = deer tan, bg = dark panel).
type spriteRig struct {
	r      *raster.Rasterizer
	e      *fake.Engine
	t      *testing.T
	fg, bg vtengine.RGB
}

func newSpriteRig(t *testing.T, cols, rows int) *spriteRig {
	t.Helper()
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	geo := vtengine.Geometry{Cols: cols, Rows: rows}
	geo.CellW, geo.CellH = r.LogicalCellSize()
	e := fake.New(vtengine.Options{Geometry: geo})
	t.Cleanup(func() { _ = e.Close() })
	fg := vtengine.RGB{R: 0xc8, G: 0xa2, B: 0x78}
	bg := vtengine.RGB{R: 0x11, G: 0x11, B: 0x13}
	e.SetColors(vtengine.Colors{FG: fg, BG: bg})
	e.SetCursor(vtengine.Cursor{Visible: false})
	return &spriteRig{r: r, e: e, t: t, fg: fg, bg: bg}
}

func (g *spriteRig) render() *image.RGBA {
	g.t.Helper()
	var f vtengine.Frame
	if err := g.e.Snapshot(&f); err != nil {
		g.t.Fatal(err)
	}
	img, err := g.r.Render(&f, g.e, nil)
	if err != nil {
		g.t.Fatal(err)
	}
	return img
}

// TestHalfBlocksTileSeamlessly is tenten's mascot bug distilled: a solid
// region built from ▀/▄/█ cells (fg = bg = one color, the half-block
// blitter's interior encoding) must come out as ONE flat rectangle —
// every stray pixel is a phantom cut line the user can see.
func TestHalfBlocksTileSeamlessly(t *testing.T) {
	g := newSpriteRig(t, 6, 4)
	tan := g.fg
	st := vtengine.Style{FG: tan, BG: tan, HasBG: true}
	glyphs := []string{"▀", "▄", "█"}
	for y := 0; y < 4; y++ {
		for x := 0; x < 6; x++ {
			g.e.SetCell(x, y, glyphs[(x+y)%3], st)
		}
	}
	img := g.render()
	cw, ch := g.r.CellSize()
	want := color.RGBA{R: tan.R, G: tan.G, B: tan.B, A: 0xff}
	for y := 0; y < 4*ch; y++ {
		for x := 0; x < 6*cw; x++ {
			if got := img.RGBAAt(x, y); got != want {
				t.Fatalf("phantom pixel at (%d,%d): got %v, want %v (seam through solid half-block art)", x, y, got, want)
			}
		}
	}
}

// TestUpperHalfBlockEdgeIsExact pins the sprite geometry itself: a ▀ with
// default background covers exactly the rounded upper half — a crisp
// horizontal edge, no antialiased rows, no gap at the cell top.
func TestUpperHalfBlockEdgeIsExact(t *testing.T) {
	g := newSpriteRig(t, 1, 1)
	g.e.SetCell(0, 0, "▀", vtengine.Style{FG: g.fg, BG: g.bg})
	img := g.render()
	cw, ch := g.r.CellSize()
	fg := color.RGBA{R: g.fg.R, G: g.fg.G, B: g.fg.B, A: 0xff}
	bg := color.RGBA{R: g.bg.R, G: g.bg.G, B: g.bg.B, A: 0xff}
	split := (ch*4 + 4) / 8 // frac(ch, 4, 8): the shared half edge
	for y := 0; y < ch; y++ {
		want := fg
		if y >= split {
			want = bg
		}
		for x := 0; x < cw; x++ {
			if got := img.RGBAAt(x, y); got != want {
				t.Fatalf("▀ pixel (%d,%d) = %v, want %v (split row %d)", x, y, got, want, split)
			}
		}
	}
}

// TestQuadrantsPartitionTheCell: the four quadrant glyphs cover every
// cell pixel exactly once (their union is █, their pairwise overlap nil).
func TestQuadrantsPartitionTheCell(t *testing.T) {
	g := newSpriteRig(t, 5, 1)
	st := vtengine.Style{FG: g.fg, BG: g.bg}
	for i, s := range []string{"▘", "▝", "▖", "▗", "█"} {
		g.e.SetCell(i, 0, s, st)
	}
	img := g.render()
	cw, ch := g.r.CellSize()
	fg := color.RGBA{R: g.fg.R, G: g.fg.G, B: g.fg.B, A: 0xff}
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			covered := 0
			for q := 0; q < 4; q++ {
				if img.RGBAAt(q*cw+x, y) == fg {
					covered++
				}
			}
			full := img.RGBAAt(4*cw+x, y) == fg
			if covered != 1 || !full {
				t.Fatalf("pixel (%d,%d): covered by %d quadrants, full block %v — want exactly 1 and true", x, y, covered, full)
			}
		}
	}
}

// TestShadesAreUniformAndMonotonic: ░▒▓ blend fg over bg at 25/50/75%,
// flat across the whole cell (no stipple seams), strictly ordered.
func TestShadesAreUniformAndMonotonic(t *testing.T) {
	g := newSpriteRig(t, 3, 1)
	st := vtengine.Style{FG: g.fg, BG: g.bg}
	for i, s := range []string{"░", "▒", "▓"} {
		g.e.SetCell(i, 0, s, st)
	}
	img := g.render()
	cw, ch := g.r.CellSize()
	var levels [3]color.RGBA
	for i := 0; i < 3; i++ {
		levels[i] = img.RGBAAt(i*cw+cw/2, ch/2)
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				if got := img.RGBAAt(i*cw+x, y); got != levels[i] {
					t.Fatalf("shade %d not uniform: (%d,%d) = %v vs %v", i, x, y, got, levels[i])
				}
			}
		}
	}
	if levels[0].R >= levels[1].R || levels[1].R >= levels[2].R {
		t.Fatalf("shades not monotonic toward fg: %v", levels)
	}
}

// TestBoxLinesConnectAcrossCells: ┌─┬─┐ over │ ┼ │ etc. — the stroke band
// must be continuously inked across every cell boundary it crosses, the
// exact property font-rendered box glyphs break.
func TestBoxLinesConnectAcrossCells(t *testing.T) {
	g := newSpriteRig(t, 5, 3)
	st := vtengine.Style{FG: g.fg, BG: g.bg}
	rows := []string{"┌─┬─┐", "├─┼─┤", "└─┴─┘"}
	for y, row := range rows {
		x := 0
		for _, rn := range row {
			g.e.SetCell(x, y, string(rn), st)
			x++
		}
	}
	img := g.render()
	cw, ch := g.r.CellSize()
	bg := color.RGBA{R: g.bg.R, G: g.bg.G, B: g.bg.B, A: 0xff}
	inked := func(x, y int) bool { return img.RGBAAt(x, y) != bg }

	// Horizontal band of each row: continuous from the first cell's
	// vertical band to the last cell's vertical band.
	bandY := func(row int) int { return row*ch + ch/2 }
	for row := 0; row < 3; row++ {
		for x := cw / 2; x <= 4*cw+cw/2; x++ {
			if !inked(x, bandY(row)) {
				t.Fatalf("row %d: horizontal stroke broken at x=%d (cell boundary gap)", row, x)
			}
		}
	}
	// Vertical bands of columns 0, 2, 4: continuous top row center to
	// bottom row center.
	for _, col := range []int{0, 2, 4} {
		bx := col*cw + cw/2
		for y := ch / 2; y <= 2*ch+ch/2; y++ {
			if !inked(bx, y) {
				t.Fatalf("col %d: vertical stroke broken at y=%d (cell boundary gap)", col, y)
			}
		}
	}
}

// TestRoundedCornersJoinStraightLines: tenten's panel border. ╭─╮ / │ │ /
// ╰─╯ must be one continuous ink path — the arc has to reach its cell
// edges on the same center bands the straight glyphs use.
func TestRoundedCornersJoinStraightLines(t *testing.T) {
	g := newSpriteRig(t, 3, 3)
	st := vtengine.Style{FG: g.fg, BG: g.bg}
	rows := []string{"╭─╮", "│ │", "╰─╯"}
	for y, row := range rows {
		x := 0
		for _, rn := range row {
			g.e.SetCell(x, y, string(rn), st)
			x++
		}
	}
	img := g.render()
	cw, ch := g.r.CellSize()
	bg := color.RGBA{R: g.bg.R, G: g.bg.G, B: g.bg.B, A: 0xff}
	inked := func(x, y int) bool { return img.RGBAAt(x, y) != bg }

	// The joins are the seam property: ink on BOTH sides of every cell
	// boundary the border crosses, at the shared stroke band.
	joins := [][4]int{
		{cw - 1, ch / 2, cw, ch / 2},                 // ╭ → ─
		{2*cw - 1, ch / 2, 2 * cw, ch / 2},           // ─ → ╮
		{cw / 2, ch - 1, cw / 2, ch},                 // ╭ → │
		{cw / 2, 2*ch - 1, cw / 2, 2 * ch},           // │ → ╰
		{2*cw + cw/2, ch - 1, 2*cw + cw/2, ch},       // ╮ → │
		{cw - 1, 2*ch + ch/2, cw, 2*ch + ch/2},       // ╰ → ─
		{2*cw - 1, 2*ch + ch/2, 2 * cw, 2*ch + ch/2}, // ─ → ╯
	}
	for _, j := range joins {
		if !inked(j[0], j[1]) || !inked(j[2], j[3]) {
			t.Fatalf("border join broken across (%d,%d)-(%d,%d)", j[0], j[1], j[2], j[3])
		}
	}
	// And ─'s own band is solid through its cell.
	for x := cw; x < 2*cw; x++ {
		if !inked(x, ch/2) {
			t.Fatalf("─ stroke broken at x=%d", x)
		}
	}
}

// TestDoubleLinesKeepTheirChannel: ╔═╗ — two parallel strokes with an
// open channel between them; the channel must stay background along the
// span, and the strokes must be continuous.
func TestDoubleLinesKeepTheirChannel(t *testing.T) {
	g := newSpriteRig(t, 3, 1)
	st := vtengine.Style{FG: g.fg, BG: g.bg}
	for i, s := range []string{"╔", "═", "╗"} {
		g.e.SetCell(i, 0, s, st)
	}
	img := g.render()
	cw, ch := g.r.CellSize()
	bg := color.RGBA{R: g.bg.R, G: g.bg.G, B: g.bg.B, A: 0xff}
	inked := func(x, y int) bool { return img.RGBAAt(x, y) != bg }

	// Find the two stroke runs down the middle of the ═ cell.
	midX := cw + cw/2
	var runs [][2]int
	for y := 0; y < ch; {
		if !inked(midX, y) {
			y++
			continue
		}
		start := y
		for y < ch && inked(midX, y) {
			y++
		}
		runs = append(runs, [2]int{start, y})
	}
	if len(runs) != 2 {
		t.Fatalf("═ has %d stroke runs at x=%d, want 2 (double line)", len(runs), midX)
	}
	// The channel between them stays open across the whole ═ cell.
	for x := cw; x < 2*cw; x++ {
		for y := runs[0][1]; y < runs[1][0]; y++ {
			if inked(x, y) {
				t.Fatalf("double-line channel filled at (%d,%d) — strokes merged", x, y)
			}
		}
	}
	// Each stroke is continuous through the ═ cell and across both cell
	// boundaries into the corners (the seam property).
	for _, run := range runs {
		for x := cw - 1; x <= 2*cw; x++ {
			if !inked(x, run[0]) {
				t.Fatalf("double stroke broken at (%d,%d)", x, run[0])
			}
		}
	}
}

// TestBrailleDots: ⣿ inks its 8 square dots, ⠀ inks nothing, and dots
// stay inside their own half-column/quarter-row so patterns read cleanly.
func TestBrailleDots(t *testing.T) {
	g := newSpriteRig(t, 2, 1)
	st := vtengine.Style{FG: g.fg, BG: g.bg}
	g.e.SetCell(0, 0, "⣿", st) // ⣿ all dots
	g.e.SetCell(1, 0, "⠀", st) // ⠀ blank
	img := g.render()
	cw, ch := g.r.CellSize()
	bg := color.RGBA{R: g.bg.R, G: g.bg.G, B: g.bg.B, A: 0xff}

	regions := 0
	for ry := 0; ry < 4; ry++ {
		for rx := 0; rx < 2; rx++ {
			found := false
			for y := ry * ch / 4; y < (ry+1)*ch/4 && !found; y++ {
				for x := rx * cw / 2; x < (rx+1)*cw/2 && !found; x++ {
					if img.RGBAAt(x, y) != bg {
						found = true
					}
				}
			}
			if found {
				regions++
			}
		}
	}
	if regions != 8 {
		t.Fatalf("⣿ inked %d of 8 dot regions", regions)
	}
	for y := 0; y < ch; y++ {
		for x := cw; x < 2*cw; x++ {
			if img.RGBAAt(x, y) != bg {
				t.Fatalf("⠀ (blank braille) inked pixel at (%d,%d)", x-cw, y)
			}
		}
	}
}

// TestEverySpriteRuneInks closes the silent-invisible hole: every rune
// foley claims as a sprite must produce ink — a dropped dispatch entry
// would otherwise render an EMPTY mask and the glyph would vanish with
// no warning. U+2800 (blank braille) is the one legitimate empty.
func TestEverySpriteRuneInks(t *testing.T) {
	const cols = 32
	g := newSpriteRig(t, cols, 1)
	st := vtengine.Style{FG: g.fg, BG: g.bg}
	bg := color.RGBA{R: g.bg.R, G: g.bg.G, B: g.bg.B, A: 0xff}
	cw, ch := g.r.CellSize()

	var runes []rune
	for rn := rune(0x2500); rn <= 0x259f; rn++ {
		runes = append(runes, rn)
	}
	for rn := rune(0x2800); rn <= 0x28ff; rn++ {
		runes = append(runes, rn)
	}
	for start := 0; start < len(runes); start += cols {
		for i := 0; i < cols; i++ {
			rn := rune('█') // pad the tail chunk with a known-inked glyph
			if start+i < len(runes) {
				rn = runes[start+i]
			}
			g.e.SetCell(i, 0, string(rn), st)
		}
		img := g.render()
		for i := 0; i < cols && start+i < len(runes); i++ {
			rn := runes[start+i]
			inked := false
			for y := 0; y < ch && !inked; y++ {
				for x := 0; x < cw && !inked; x++ {
					if img.RGBAAt(i*cw+x, y) != bg {
						inked = true
					}
				}
			}
			if want := rn != 0x2800; inked != want {
				t.Fatalf("U+%04X %q: inked=%v, want %v", rn, string(rn), inked, want)
			}
		}
	}
}

// TestBlocksShareEdgesAtOddCells pins the parada finding: at Scale 1
// cells can be odd, and the half blocks must share their split edge with
// the quadrants — ▀ ends exactly where ▄ and ▟'s lower half begin, ▐
// begins where ▝'s right half does.
func TestBlocksShareEdgesAtOddCells(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	white := vtengine.RGB{R: 0xff, G: 0xff, B: 0xff}
	for _, size := range []int{12, 13, 14, 15, 16, 17} {
		r, err := raster.New(raster.Options{Pack: pack, FontSizePx: size, Scale: 1})
		if err != nil {
			t.Fatal(err)
		}
		cw, ch := r.CellSize()
		geo := vtengine.Geometry{Cols: 4, Rows: 1, CellW: cw, CellH: ch}
		e := fake.New(vtengine.Options{Geometry: geo})
		e.SetColors(vtengine.Colors{FG: white, BG: vtengine.RGB{}})
		e.SetCursor(vtengine.Cursor{Visible: false})
		for i, s := range []string{"▀", "▄", "▟", "▐"} {
			e.SetCell(i, 0, s, vtengine.Style{FG: white})
		}
		var f vtengine.Frame
		if err := e.Snapshot(&f); err != nil {
			t.Fatal(err)
		}
		img, err := r.Render(&f, e, nil)
		if err != nil {
			t.Fatal(err)
		}
		fg := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		inkEndRow := func(x int) int { // first non-inked row (▀)
			for y := 0; y < ch; y++ {
				if img.RGBAAt(x, y) != fg {
					return y
				}
			}
			return ch
		}
		inkStartRow := func(x int) int {
			for y := 0; y < ch; y++ {
				if img.RGBAAt(x, y) == fg {
					return y
				}
			}
			return -1
		}
		inkStartCol := func(cell, y int) int {
			for x := 0; x < cw; x++ {
				if img.RGBAAt(cell*cw+x, y) == fg {
					return x
				}
			}
			return -1
		}
		upperEnd := inkEndRow(cw / 4)              // ▀
		lowerStart := inkStartRow(cw + cw/4)       // ▄
		quadLowerStart := inkStartRow(2*cw + cw/4) // ▟'s lower-left quadrant
		if upperEnd != lowerStart || lowerStart != quadLowerStart {
			t.Fatalf("size %d (cell %dx%d): split edges differ — ▀ ends %d, ▄ starts %d, ▟ starts %d",
				size, cw, ch, upperEnd, lowerStart, quadLowerStart)
		}
		rightStart := inkStartCol(3, ch/4) // ▐ at a top row
		if xm := (cw*4 + 4) / 8; rightStart != xm {
			t.Fatalf("size %d (cell %dx%d): ▐ starts col %d, want shared edge %d", size, cw, ch, rightStart, xm)
		}
		_ = e.Close()
	}
}

// TestSpritesIgnoreFontStyle: bold ▀ must be the same sprite as plain ▀
// (terminals never embolden sprites) — and must not join a text shaping
// run with neighboring letters.
func TestSpritesIgnoreFontStyle(t *testing.T) {
	g := newSpriteRig(t, 3, 2)
	g.e.SetCell(0, 0, "a", vtengine.Style{FG: g.fg, Bold: true})
	g.e.SetCell(1, 0, "▀", vtengine.Style{FG: g.fg, Bold: true})
	g.e.SetCell(2, 0, "b", vtengine.Style{FG: g.fg, Bold: true})
	g.e.SetCell(1, 1, "▀", vtengine.Style{FG: g.fg})
	img := g.render()
	cw, ch := g.r.CellSize()
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			if img.RGBAAt(cw+x, y) != img.RGBAAt(cw+x, ch+y) {
				t.Fatalf("bold ▀ differs from plain ▀ at (%d,%d)", x, y)
			}
		}
	}
}
