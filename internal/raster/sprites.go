package raster

import (
	"image"

	"github.com/GH-Jaider/foley/internal/vtengine"
)

// Sprite glyphs: Box Drawing (U+2500–257F), Block Elements (U+2580–259F)
// and Braille Patterns (U+2800–28FF) are SYNTHESIZED from cell geometry
// instead of rasterized from the font. Every real terminal (kitty,
// ghostty, alacritty, wezterm) does this, because font outlines never
// tile exactly: sub-pixel gaps and antialiased edges between adjacent
// cells read as phantom cut lines through solid half-block pixel art
// (found live by tenten's studio mascots). Geometry mirrors ghostty's
// sprite font (MIT — src/font/sprite/draw/{box,block,braille}.zig), the
// same project as our VT engine.
//
// Determinism: rectangles, dashes, blocks and braille are integer-only.
// Arcs and diagonals flatten to segments and supersample coverage at
// 4×4 in float64 — only exactly-rounded ops and compares, with the same
// explicit-conversion FMA barrier discipline as mask() in text.go.

// isSpriteRune reports whether the rune renders as a synthesized sprite
// (single-rune cells only; sprite codepoints never carry combining marks).
func isSpriteRune(rn rune) bool {
	return (rn >= 0x2500 && rn <= 0x259f) || (rn >= 0x2800 && rn <= 0x28ff)
}

func spriteCell(c *vtengine.Cell) bool {
	return len(c.Runes) == 1 && isSpriteRune(c.Runes[0])
}

func (r *Rasterizer) drawSpriteCell(dst *image.RGBA, f *vtengine.Frame, x, y int) {
	cell := f.CellAt(x, y)
	bg, fg := effectiveColors(cell.Style, f)
	if cell.Style.Faint {
		fg = mix(fg, bg)
	}
	blitMask(dst, r.spriteMask(cell.Runes[0]), r.orgX+x*r.cellW, r.orgY+y*r.cellH, fg)
	r.drawDecorations(dst, f, x, y)
}

// spriteMask renders (and caches) the full-cell alpha mask of a sprite.
func (r *Rasterizer) spriteMask(rn rune) *glyphMask {
	if m, ok := r.sprites[rn]; ok {
		return m
	}
	a := image.NewAlpha(image.Rect(0, 0, r.cellW, r.cellH))
	switch {
	case rn >= 0x2500 && rn <= 0x257f:
		r.spriteBox(a, rn)
	case rn >= 0x2580 && rn <= 0x259f:
		r.spriteBlock(a, rn)
	default:
		r.spriteBraille(a, rn)
	}
	m := &glyphMask{alpha: a} // left/top zero: blit anchors at the cell origin
	r.sprites[rn] = m
	return m
}

// fillAlpha paints a clipped rectangle of coverage, keeping the max where
// shapes overlap (double-line strokes cross their perpendiculars).
func fillAlpha(a *image.Alpha, x0, y0, x1, y1 int, v uint8) {
	b := a.Bounds()
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = min(x1, b.Max.X), min(y1, b.Max.Y)
	for y := y0; y < y1; y++ {
		row := a.Pix[y*a.Stride+x0 : y*a.Stride+x1]
		for i := range row {
			if row[i] < v {
				row[i] = v
			}
		}
	}
}

// frac returns round(size·num/den). Every glyph that shares an art edge
// anchors to this same rounded value — the half split in particular
// (▀/▄/▌/▐ and the quadrants) — so those partitions tile exactly even at
// odd sizes. Eighth-anchored blocks pair only with their own complements
// (▔ with ▇, ▏ with ▊…), where a rounding tie overlaps 1px, never gaps.
func frac(size, num, den int) int { return (size*num + den/2) / den }

// ── Block Elements U+2580–259F ─────────────────────────────────────────

func (r *Rasterizer) spriteBlock(a *image.Alpha, rn rune) {
	w, h := r.cellW, r.cellH
	lower := func(eighths int) { fillAlpha(a, 0, h-frac(h, eighths, 8), w, h, 0xff) }
	left := func(eighths int) { fillAlpha(a, 0, 0, frac(w, eighths, 8), h, 0xff) }
	xm, ym := frac(w, 4, 8), frac(h, 4, 8)
	quad := func(tl, tr, bl, br bool) {
		if tl {
			fillAlpha(a, 0, 0, xm, ym, 0xff)
		}
		if tr {
			fillAlpha(a, xm, 0, w, ym, 0xff)
		}
		if bl {
			fillAlpha(a, 0, ym, xm, h, 0xff)
		}
		if br {
			fillAlpha(a, xm, ym, w, h, 0xff)
		}
	}
	switch rn {
	case '▀':
		fillAlpha(a, 0, 0, w, ym, 0xff)
	case '▁':
		lower(1)
	case '▂':
		lower(2)
	case '▃':
		lower(3)
	case '▄':
		// The half block anchors to the SHARED split edge ym, not to a
		// bottom-anchored rounding of its own: at odd heights they
		// differ by 1px and ▄ would jag against ▟/▙ (parada finding).
		fillAlpha(a, 0, ym, w, h, 0xff)
	case '▅':
		lower(5)
	case '▆':
		lower(6)
	case '▇':
		lower(7)
	case '█':
		fillAlpha(a, 0, 0, w, h, 0xff)
	case '▉':
		left(7)
	case '▊':
		left(6)
	case '▋':
		left(5)
	case '▌':
		left(4)
	case '▍':
		left(3)
	case '▎':
		left(2)
	case '▏':
		left(1)
	case '▐':
		fillAlpha(a, xm, 0, w, h, 0xff) // shared split edge, like ▄
	case '░':
		fillAlpha(a, 0, 0, w, h, 0x40)
	case '▒':
		fillAlpha(a, 0, 0, w, h, 0x80)
	case '▓':
		fillAlpha(a, 0, 0, w, h, 0xc0)
	case '▔':
		fillAlpha(a, 0, 0, w, frac(h, 1, 8), 0xff)
	case '▕':
		fillAlpha(a, w-frac(w, 1, 8), 0, w, h, 0xff)
	case '▖':
		quad(false, false, true, false)
	case '▗':
		quad(false, false, false, true)
	case '▘':
		quad(true, false, false, false)
	case '▙':
		quad(true, false, true, true)
	case '▚':
		quad(true, false, false, true)
	case '▛':
		quad(true, true, true, false)
	case '▜':
		quad(true, true, false, true)
	case '▝':
		quad(false, true, false, false)
	case '▞':
		quad(false, true, true, false)
	case '▟':
		quad(false, true, true, true)
	}
}

// ── Box Drawing U+2500–257F ────────────────────────────────────────────

type lineWeight uint8

const (
	wNone lineWeight = iota
	wLight
	wHeavy
	wDouble
)

// boxLines is one glyph's arm weights toward each cell edge.
type boxLines struct{ up, right, down, left lineWeight }

func (r *Rasterizer) spriteBox(a *image.Alpha, rn rune) {
	t := r.thickness
	switch rn {
	case '┄':
		r.drawDash(a, true, 3, t)
	case '┅':
		r.drawDash(a, true, 3, 2*t)
	case '┆':
		r.drawDash(a, false, 3, t)
	case '┇':
		r.drawDash(a, false, 3, 2*t)
	case '┈':
		r.drawDash(a, true, 4, t)
	case '┉':
		r.drawDash(a, true, 4, 2*t)
	case '┊':
		r.drawDash(a, false, 4, t)
	case '┋':
		r.drawDash(a, false, 4, 2*t)
	case '╌':
		r.drawDash(a, true, 2, t)
	case '╍':
		r.drawDash(a, true, 2, 2*t)
	case '╎':
		r.drawDash(a, false, 2, t)
	case '╏':
		r.drawDash(a, false, 2, 2*t)
	case '╭', '╮', '╯', '╰':
		r.drawArc(a, rn)
	case '╱':
		r.drawDiagonals(a, false, true)
	case '╲':
		r.drawDiagonals(a, true, false)
	case '╳':
		r.drawDiagonals(a, true, true)
	default:
		if ln, ok := boxLinesFor(rn); ok {
			r.drawBoxLines(a, ln)
		}
	}
}

// boxLinesFor mirrors ghostty's dispatch table one-to-one
// (src/font/sprite/draw/box.zig). Dashes, arcs and diagonals are handled
// separately; every other U+2500–257F glyph is a combination of arms.
func boxLinesFor(rn rune) (boxLines, bool) {
	const L, H, D = wLight, wHeavy, wDouble
	switch rn {
	case '─':
		return boxLines{left: L, right: L}, true
	case '━':
		return boxLines{left: H, right: H}, true
	case '│':
		return boxLines{up: L, down: L}, true
	case '┃':
		return boxLines{up: H, down: H}, true
	case '┌':
		return boxLines{down: L, right: L}, true
	case '┍':
		return boxLines{down: L, right: H}, true
	case '┎':
		return boxLines{down: H, right: L}, true
	case '┏':
		return boxLines{down: H, right: H}, true
	case '┐':
		return boxLines{down: L, left: L}, true
	case '┑':
		return boxLines{down: L, left: H}, true
	case '┒':
		return boxLines{down: H, left: L}, true
	case '┓':
		return boxLines{down: H, left: H}, true
	case '└':
		return boxLines{up: L, right: L}, true
	case '┕':
		return boxLines{up: L, right: H}, true
	case '┖':
		return boxLines{up: H, right: L}, true
	case '┗':
		return boxLines{up: H, right: H}, true
	case '┘':
		return boxLines{up: L, left: L}, true
	case '┙':
		return boxLines{up: L, left: H}, true
	case '┚':
		return boxLines{up: H, left: L}, true
	case '┛':
		return boxLines{up: H, left: H}, true
	case '├':
		return boxLines{up: L, down: L, right: L}, true
	case '┝':
		return boxLines{up: L, down: L, right: H}, true
	case '┞':
		return boxLines{up: H, down: L, right: L}, true
	case '┟':
		return boxLines{up: L, down: H, right: L}, true
	case '┠':
		return boxLines{up: H, down: H, right: L}, true
	case '┡':
		return boxLines{up: H, down: L, right: H}, true
	case '┢':
		return boxLines{up: L, down: H, right: H}, true
	case '┣':
		return boxLines{up: H, down: H, right: H}, true
	case '┤':
		return boxLines{up: L, down: L, left: L}, true
	case '┥':
		return boxLines{up: L, down: L, left: H}, true
	case '┦':
		return boxLines{up: H, down: L, left: L}, true
	case '┧':
		return boxLines{up: L, down: H, left: L}, true
	case '┨':
		return boxLines{up: H, down: H, left: L}, true
	case '┩':
		return boxLines{up: H, down: L, left: H}, true
	case '┪':
		return boxLines{up: L, down: H, left: H}, true
	case '┫':
		return boxLines{up: H, down: H, left: H}, true
	case '┬':
		return boxLines{down: L, left: L, right: L}, true
	case '┭':
		return boxLines{down: L, left: H, right: L}, true
	case '┮':
		return boxLines{down: L, left: L, right: H}, true
	case '┯':
		return boxLines{down: L, left: H, right: H}, true
	case '┰':
		return boxLines{down: H, left: L, right: L}, true
	case '┱':
		return boxLines{down: H, left: H, right: L}, true
	case '┲':
		return boxLines{down: H, left: L, right: H}, true
	case '┳':
		return boxLines{down: H, left: H, right: H}, true
	case '┴':
		return boxLines{up: L, left: L, right: L}, true
	case '┵':
		return boxLines{up: L, left: H, right: L}, true
	case '┶':
		return boxLines{up: L, left: L, right: H}, true
	case '┷':
		return boxLines{up: L, left: H, right: H}, true
	case '┸':
		return boxLines{up: H, left: L, right: L}, true
	case '┹':
		return boxLines{up: H, left: H, right: L}, true
	case '┺':
		return boxLines{up: H, left: L, right: H}, true
	case '┻':
		return boxLines{up: H, left: H, right: H}, true
	case '┼':
		return boxLines{up: L, down: L, left: L, right: L}, true
	case '┽':
		return boxLines{up: L, down: L, left: H, right: L}, true
	case '┾':
		return boxLines{up: L, down: L, left: L, right: H}, true
	case '┿':
		return boxLines{up: L, down: L, left: H, right: H}, true
	case '╀':
		return boxLines{up: H, down: L, left: L, right: L}, true
	case '╁':
		return boxLines{up: L, down: H, left: L, right: L}, true
	case '╂':
		return boxLines{up: H, down: H, left: L, right: L}, true
	case '╃':
		return boxLines{up: H, down: L, left: H, right: L}, true
	case '╄':
		return boxLines{up: H, down: L, left: L, right: H}, true
	case '╅':
		return boxLines{up: L, down: H, left: H, right: L}, true
	case '╆':
		return boxLines{up: L, down: H, left: L, right: H}, true
	case '╇':
		return boxLines{up: H, down: L, left: H, right: H}, true
	case '╈':
		return boxLines{up: L, down: H, left: H, right: H}, true
	case '╉':
		return boxLines{up: H, down: H, left: H, right: L}, true
	case '╊':
		return boxLines{up: H, down: H, left: L, right: H}, true
	case '╋':
		return boxLines{up: H, down: H, left: H, right: H}, true
	case '═':
		return boxLines{left: D, right: D}, true
	case '║':
		return boxLines{up: D, down: D}, true
	case '╒':
		return boxLines{down: L, right: D}, true
	case '╓':
		return boxLines{down: D, right: L}, true
	case '╔':
		return boxLines{down: D, right: D}, true
	case '╕':
		return boxLines{down: L, left: D}, true
	case '╖':
		return boxLines{down: D, left: L}, true
	case '╗':
		return boxLines{down: D, left: D}, true
	case '╘':
		return boxLines{up: L, right: D}, true
	case '╙':
		return boxLines{up: D, right: L}, true
	case '╚':
		return boxLines{up: D, right: D}, true
	case '╛':
		return boxLines{up: L, left: D}, true
	case '╜':
		return boxLines{up: D, left: L}, true
	case '╝':
		return boxLines{up: D, left: D}, true
	case '╞':
		return boxLines{up: L, down: L, right: D}, true
	case '╟':
		return boxLines{up: D, down: D, right: L}, true
	case '╠':
		return boxLines{up: D, down: D, right: D}, true
	case '╡':
		return boxLines{up: L, down: L, left: D}, true
	case '╢':
		return boxLines{up: D, down: D, left: L}, true
	case '╣':
		return boxLines{up: D, down: D, left: D}, true
	case '╤':
		return boxLines{down: L, left: D, right: D}, true
	case '╥':
		return boxLines{down: D, left: L, right: L}, true
	case '╦':
		return boxLines{down: D, left: D, right: D}, true
	case '╧':
		return boxLines{up: L, left: D, right: D}, true
	case '╨':
		return boxLines{up: D, left: L, right: L}, true
	case '╩':
		return boxLines{up: D, left: D, right: D}, true
	case '╪':
		return boxLines{up: L, down: L, left: D, right: D}, true
	case '╫':
		return boxLines{up: D, down: D, left: L, right: L}, true
	case '╬':
		return boxLines{up: D, down: D, left: D, right: D}, true
	case '╴':
		return boxLines{left: L}, true
	case '╵':
		return boxLines{up: L}, true
	case '╶':
		return boxLines{right: L}, true
	case '╷':
		return boxLines{down: L}, true
	case '╸':
		return boxLines{left: H}, true
	case '╹':
		return boxLines{up: H}, true
	case '╺':
		return boxLines{right: H}, true
	case '╻':
		return boxLines{down: H}, true
	case '╼':
		return boxLines{left: L, right: H}, true
	case '╽':
		return boxLines{up: L, down: H}, true
	case '╾':
		return boxLines{left: H, right: L}, true
	case '╿':
		return boxLines{up: H, down: L}, true
	}
	return boxLines{}, false
}

// drawBoxLines is ghostty's linesChar: per-arm strokes whose extents
// depend on the perpendicular arms, so junctions join without overdraw
// and double strokes leave their channel open where it must flow.
func (r *Rasterizer) drawBoxLines(a *image.Alpha, ln boxLines) {
	w, h := r.cellW, r.cellH
	t := r.thickness
	th := 2 * t

	hLightTop := (h - t) / 2
	hLightBot := hLightTop + t
	hHeavyTop := (h - th) / 2
	hHeavyBot := hHeavyTop + th
	hDoubleTop := hLightTop - t
	hDoubleBot := hLightBot + t
	vLightLeft := (w - t) / 2
	vLightRight := vLightLeft + t
	vHeavyLeft := (w - th) / 2
	vHeavyRight := vHeavyLeft + th
	vDoubleLeft := vLightLeft - t
	vDoubleRight := vLightRight + t

	var upBottom int
	switch {
	case ln.left == wHeavy || ln.right == wHeavy:
		upBottom = hHeavyBot
	case ln.left != ln.right || ln.down == ln.up:
		if ln.left == wDouble || ln.right == wDouble {
			upBottom = hDoubleBot
		} else {
			upBottom = hLightBot
		}
	case ln.left == wNone && ln.right == wNone:
		upBottom = hLightBot
	default:
		upBottom = hLightTop
	}

	var downTop int
	switch {
	case ln.left == wHeavy || ln.right == wHeavy:
		downTop = hHeavyTop
	case ln.left != ln.right || ln.up == ln.down:
		if ln.left == wDouble || ln.right == wDouble {
			downTop = hDoubleTop
		} else {
			downTop = hLightTop
		}
	case ln.left == wNone && ln.right == wNone:
		downTop = hLightTop
	default:
		downTop = hLightBot
	}

	var leftRight int
	switch {
	case ln.up == wHeavy || ln.down == wHeavy:
		leftRight = vHeavyRight
	case ln.up != ln.down || ln.left == ln.right:
		if ln.up == wDouble || ln.down == wDouble {
			leftRight = vDoubleRight
		} else {
			leftRight = vLightRight
		}
	case ln.up == wNone && ln.down == wNone:
		leftRight = vLightRight
	default:
		leftRight = vLightLeft
	}

	var rightLeft int
	switch {
	case ln.up == wHeavy || ln.down == wHeavy:
		rightLeft = vHeavyLeft
	case ln.up != ln.down || ln.right == ln.left:
		if ln.up == wDouble || ln.down == wDouble {
			rightLeft = vDoubleLeft
		} else {
			rightLeft = vLightLeft
		}
	case ln.up == wNone && ln.down == wNone:
		rightLeft = vLightLeft
	default:
		rightLeft = vLightRight
	}

	switch ln.up {
	case wNone:
	case wLight:
		fillAlpha(a, vLightLeft, 0, vLightRight, upBottom, 0xff)
	case wHeavy:
		fillAlpha(a, vHeavyLeft, 0, vHeavyRight, upBottom, 0xff)
	case wDouble:
		lb, rb := upBottom, upBottom
		if ln.left == wDouble {
			lb = hLightTop
		}
		if ln.right == wDouble {
			rb = hLightTop
		}
		fillAlpha(a, vDoubleLeft, 0, vLightLeft, lb, 0xff)
		fillAlpha(a, vLightRight, 0, vDoubleRight, rb, 0xff)
	}

	switch ln.down {
	case wNone:
	case wLight:
		fillAlpha(a, vLightLeft, downTop, vLightRight, h, 0xff)
	case wHeavy:
		fillAlpha(a, vHeavyLeft, downTop, vHeavyRight, h, 0xff)
	case wDouble:
		lt, rt := downTop, downTop
		if ln.left == wDouble {
			lt = hLightBot
		}
		if ln.right == wDouble {
			rt = hLightBot
		}
		fillAlpha(a, vDoubleLeft, lt, vLightLeft, h, 0xff)
		fillAlpha(a, vLightRight, rt, vDoubleRight, h, 0xff)
	}

	switch ln.left {
	case wNone:
	case wLight:
		fillAlpha(a, 0, hLightTop, leftRight, hLightBot, 0xff)
	case wHeavy:
		fillAlpha(a, 0, hHeavyTop, leftRight, hHeavyBot, 0xff)
	case wDouble:
		tr, br := leftRight, leftRight
		if ln.up == wDouble {
			tr = vLightLeft
		}
		if ln.down == wDouble {
			br = vLightLeft
		}
		fillAlpha(a, 0, hDoubleTop, tr, hLightTop, 0xff)
		fillAlpha(a, 0, hLightBot, br, hDoubleBot, 0xff)
	}

	switch ln.right {
	case wNone:
	case wLight:
		fillAlpha(a, rightLeft, hLightTop, w, hLightBot, 0xff)
	case wHeavy:
		fillAlpha(a, rightLeft, hHeavyTop, w, hHeavyBot, 0xff)
	case wDouble:
		tl, bl := rightLeft, rightLeft
		if ln.up == wDouble {
			tl = vLightRight
		}
		if ln.down == wDouble {
			bl = vLightRight
		}
		fillAlpha(a, tl, hDoubleTop, w, hLightTop, 0xff)
		fillAlpha(a, bl, hLightBot, w, hDoubleBot, 0xff)
	}
}

// drawDash: N dashes with N gaps (half a gap at each edge), so tiled
// cells form one even dashed line. Leftover pixels widen dashes, never
// gaps. Cells too small for the pattern get a solid light line.
func (r *Rasterizer) drawDash(a *image.Alpha, horizontal bool, count, t int) {
	w, h := r.cellW, r.cellH
	span := w
	if !horizontal {
		span = h
	}
	if span < 2*count {
		if horizontal {
			fillAlpha(a, 0, (h-r.thickness)/2, w, (h-r.thickness)/2+r.thickness, 0xff)
		} else {
			fillAlpha(a, (w-r.thickness)/2, 0, (w-r.thickness)/2+r.thickness, h, 0xff)
		}
		return
	}
	gap := min(t, span/(2*count))
	totalDash := span - count*gap
	dashW := totalDash / count
	extra := totalDash % count
	pos := gap / 2
	for i := 0; i < count; i++ {
		end := pos + dashW
		if extra > 0 {
			extra--
			end++
		}
		if horizontal {
			fillAlpha(a, pos, (h-t)/2, end, (h-t)/2+t, 0xff)
		} else {
			fillAlpha(a, (w-t)/2, pos, (w-t)/2+t, end, 0xff)
		}
		pos = end + gap
	}
}

// ── Arcs and diagonals (supersampled strokes) ──────────────────────────

type spritePt struct{ x, y float64 }

// drawArc renders the rounded corners ╭╮╯╰: straight runs along the
// center bands plus ghostty's cubic (control points at s=0.25·r), so the
// arc butt-joins ─ and │ of neighboring cells exactly.
func (r *Rasterizer) drawArc(a *image.Alpha, rn rune) {
	w, h := r.cellW, r.cellH
	t := r.thickness
	cx := float64((w-t)/2) + float64(t)/2
	cy := float64((h-t)/2) + float64(t)/2
	rr := float64(min(w, h)) / 2
	const s = 0.25

	var poly []spritePt
	curve := func(p0, c1, c2, p3 spritePt) {
		const steps = 24
		for i := 1; i <= steps; i++ {
			ft := float64(i) / steps
			u := 1 - ft
			b0 := float64(u * u * u)
			b1 := float64(3 * u * u * ft)
			b2 := float64(3 * u * ft * ft)
			b3 := float64(ft * ft * ft)
			poly = append(poly, spritePt{
				x: float64(b0*p0.x) + float64(b1*c1.x) + float64(b2*c2.x) + float64(b3*p3.x),
				y: float64(b0*p0.y) + float64(b1*c1.y) + float64(b2*c2.y) + float64(b3*p3.y),
			})
		}
	}
	switch rn {
	case '╯': // arc to top-left
		poly = []spritePt{{cx, 0}, {cx, cy - rr}}
		curve(spritePt{cx, cy - rr}, spritePt{cx, cy - s*rr}, spritePt{cx - s*rr, cy}, spritePt{cx - rr, cy})
		poly = append(poly, spritePt{0, cy})
	case '╰': // arc to top-right
		poly = []spritePt{{cx, 0}, {cx, cy - rr}}
		curve(spritePt{cx, cy - rr}, spritePt{cx, cy - s*rr}, spritePt{cx + s*rr, cy}, spritePt{cx + rr, cy})
		poly = append(poly, spritePt{float64(w), cy})
	case '╮': // arc to bottom-left
		poly = []spritePt{{cx, float64(h)}, {cx, cy + rr}}
		curve(spritePt{cx, cy + rr}, spritePt{cx, cy + s*rr}, spritePt{cx - s*rr, cy}, spritePt{cx - rr, cy})
		poly = append(poly, spritePt{0, cy})
	case '╭': // arc to bottom-right
		poly = []spritePt{{cx, float64(h)}, {cx, cy + rr}}
		curve(spritePt{cx, cy + rr}, spritePt{cx, cy + s*rr}, spritePt{cx + s*rr, cy}, spritePt{cx + rr, cy})
		poly = append(poly, spritePt{float64(w), cy})
	}
	strokeAlpha(a, [][]spritePt{poly}, float64(t))
}

func (r *Rasterizer) drawDiagonals(a *image.Alpha, tlbr, trbl bool) {
	w, h := float64(r.cellW), float64(r.cellH)
	var polys [][]spritePt
	if tlbr {
		polys = append(polys, []spritePt{{0, 0}, {w, h}})
	}
	if trbl {
		polys = append(polys, []spritePt{{w, 0}, {0, h}})
	}
	strokeAlpha(a, polys, float64(r.thickness))
}

// strokeAlpha rasterizes stroked polylines by 4×4 supersampled coverage:
// a subsample is inked when its distance to any segment is at most half
// the stroke width. Products are rounded through explicit float64
// conversions (the FMA barrier — see the package determinism rule).
func strokeAlpha(a *image.Alpha, polys [][]spritePt, width float64) {
	half2 := float64((width / 2) * (width / 2))
	b := a.Bounds()
	for py := 0; py < b.Max.Y; py++ {
		for px := 0; px < b.Max.X; px++ {
			cnt := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					x := float64(px) + float64(2*sx+1)/8
					y := float64(py) + float64(2*sy+1)/8
					if withinStroke(polys, x, y, half2) {
						cnt++
					}
				}
			}
			if cnt > 0 {
				v := uint8((cnt*255 + 8) / 16) //nolint:gosec // cnt<=16 by construction
				if a.Pix[py*a.Stride+px] < v {
					a.Pix[py*a.Stride+px] = v
				}
			}
		}
	}
}

func withinStroke(polys [][]spritePt, x, y, half2 float64) bool {
	for _, poly := range polys {
		for i := 0; i+1 < len(poly); i++ {
			ax, ay := poly[i].x, poly[i].y
			dx, dy := poly[i+1].x-ax, poly[i+1].y-ay
			px, py := x-ax, y-ay
			den := float64(dx*dx) + float64(dy*dy)
			var ft float64
			if den > 0 {
				ft = (float64(px*dx) + float64(py*dy)) / den
			}
			ft = min(max(ft, 0), 1)
			ex := px - float64(ft*dx)
			ey := py - float64(ft*dy)
			if float64(ex*ex)+float64(ey*ey) <= half2 {
				return true
			}
		}
	}
	return false
}

// ── Braille Patterns U+2800–28FF ───────────────────────────────────────

// spriteBraille ports ghostty's greedy layout: square dots on a 2×4 grid,
// leftover pixels spent on (in order) dot size, margins, spacing, again
// margins, again dot size — so every cell size gets balanced dots.
func (r *Rasterizer) spriteBraille(a *image.Alpha, rn rune) {
	cw, ch := r.cellW, r.cellH
	w := min(cw/4, ch/8)
	xSp, ySp := cw/4, ch/8
	xM, yM := xSp/2, ySp/2
	xLeft := cw - 2*xM - xSp - 2*w
	yLeft := ch - 2*yM - 3*ySp - 4*w

	if xLeft >= 2 && yLeft >= 4 && w == 0 {
		w++
		xLeft -= 2
		yLeft -= 4
	}
	if xLeft >= 2 && xM == 0 {
		xM = 1
		xLeft -= 2
	}
	if yLeft >= 2 && yM == 0 {
		yM = 1
		yLeft -= 2
	}
	if xLeft >= 1 {
		xSp++
		xLeft--
	}
	if yLeft >= 3 {
		ySp++
		yLeft -= 3
	}
	if xLeft >= 2 {
		xM++
		xLeft -= 2
	}
	if yLeft >= 2 {
		yM++
		yLeft -= 2
	}
	if xLeft >= 2 && yLeft >= 4 {
		w++
	}
	if w == 0 {
		return // cell too small for any dot (never with our font sizes)
	}

	xs := [2]int{xM, xM + w + xSp}
	ys := [4]int{yM, yM + (w + ySp), yM + 2*(w+ySp), yM + 3*(w+ySp)}
	// Unicode braille bit order: tl,ul,ll,tr,ur,lr,bl,br.
	dots := [8][2]int{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 1}, {1, 2}, {0, 3}, {1, 3}}
	bits := rn - 0x2800
	for i, d := range dots {
		if bits&(1<<i) != 0 {
			fillAlpha(a, xs[d[0]], ys[d[1]], xs[d[0]]+w, ys[d[1]]+w, 0xff)
		}
	}
}
