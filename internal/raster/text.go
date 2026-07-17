package raster

import (
	"image"
	"image/color"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"github.com/GH-Jaider/foley/internal/vtengine"
)

// glyphMask is a cached alpha rendering of one glyph plus its bearings
// relative to (pen x, baseline).
type glyphMask struct {
	alpha *image.Alpha
	left  int
	top   int
}

// gridFace is the face the grid derives from: the user regular when one
// is loaded (ADR-015 — metrics follow the primary), the pack otherwise.
// A primary without basic latin (no 'M') cannot drive cell metrics or
// title shaping — the pack takes over and checkUserFont warns.
func (r *Rasterizer) gridFace() *font.Face {
	if r.user[0] != nil {
		if _, ok := r.user[0].NominalGlyph('M'); ok {
			return r.user[0]
		}
	}
	return r.text
}

func (r *Rasterizer) computeMetrics() {
	out := r.shape(r.gridFace(), []rune{'M'})
	r.cellW = out.Glyphs[0].Advance.Round()
	asc := out.LineBounds.Ascent.Round()
	desc := -out.LineBounds.Descent.Round() // Descent is negative-down in typesetting
	gap := out.LineBounds.Gap.Round()
	r.cellH = asc + desc + gap
	// Cells must be exact multiples of Scale: the geometry the app sees
	// is LOGICAL (cell/Scale via winsize) and kitty placements come back
	// in those logical pixels — an odd scaled cell makes logical*Scale
	// fall 1px short PER ROW, slicing seams through row-strip images
	// (found live by tenten's studio demo). Round UP: growing a cell
	// never clips glyphs.
	r.cellW += (r.opts.Scale - r.cellW%r.opts.Scale) % r.opts.Scale
	r.cellH += (r.opts.Scale - r.cellH%r.opts.Scale) % r.opts.Scale
	r.baseline = asc + gap/2
	r.underline = r.baseline + max(2*r.opts.Scale, desc/2)
	r.thickness = max(r.opts.Scale, r.sizePx/16)
}

func (r *Rasterizer) shape(face *font.Face, runes []rune) shaping.Output {
	return r.shapeAt(face, runes, r.sizePx)
}

// shapeAt shapes at an explicit pixel size — the grid uses sizePx; the
// window-bar title uses a bar-derived size.
func (r *Rasterizer) shapeAt(face *font.Face, runes []rune, px int) shaping.Output {
	return r.shaper.Shape(shaping.Input{
		Text: runes, RunStart: 0, RunEnd: len(runes),
		Face: face, Size: fixed.I(px),
		Script: language.Latin, Language: language.NewLanguage("en"),
	})
}

// styleIdx maps a cell style to its slot (regular, bold, italic,
// bold-italic) — the order of Rasterizer.user and styleNames.
func styleIdx(st vtengine.Style) int {
	switch {
	case st.Bold && st.Italic:
		return 3
	case st.Bold:
		return 1
	case st.Italic:
		return 2
	default:
		return 0
	}
}

// pickFace maps a style to the PACK's face for it.
func (r *Rasterizer) pickFace(st vtengine.Style) *font.Face {
	switch styleIdx(st) {
	case 3:
		return r.boldItalic
	case 1:
		return r.bold
	case 2:
		return r.italic
	default:
		return r.text
	}
}

// pickCellFace resolves the face a text cell draws with. With a user
// font loaded it is the user face for the cell's style — unless that
// face lacks the cell's base rune and the pack covers it: coverage
// falls back per cell to the pinned faces (ADR-015), style kept.
func (r *Rasterizer) pickCellFace(c *vtengine.Cell) *font.Face {
	if r.user[0] == nil {
		return r.pickFace(c.Style)
	}
	uf := r.user[styleIdx(c.Style)]
	if len(c.Runes) > 0 {
		if _, ok := uf.NominalGlyph(c.Runes[0]); !ok {
			if _, ok := r.pickFace(c.Style).NominalGlyph(c.Runes[0]); ok {
				return r.pickFace(c.Style)
			}
		}
	}
	return uf
}

// isEmojiCell: no text face can render the base rune but the emoji
// face can. With a user font, "text" means user regular OR pack.
func (r *Rasterizer) isEmojiCell(runes []rune) bool {
	if len(runes) == 0 {
		return false
	}
	if r.user[0] != nil {
		if _, ok := r.user[0].NominalGlyph(runes[0]); ok {
			return false
		}
	}
	if _, ok := r.text.NominalGlyph(runes[0]); ok {
		return false
	}
	_, ok := r.emoji.NominalGlyph(runes[0])
	return ok
}

func (r *Rasterizer) drawText(dst *image.RGBA, f *vtengine.Frame) {
	for y := 0; y < f.Geometry.Rows; y++ {
		x := 0
		for x < f.Geometry.Cols {
			cell := f.CellAt(x, y)
			switch {
			case len(cell.Runes) == 0:
				x++
			case spriteCell(cell):
				r.drawSpriteCell(dst, f, x, y)
				x++
			case r.isEmojiCell(cell.Runes):
				r.drawEmojiCell(dst, f, x, y)
				x += max(int(cell.Width), 1)
			default:
				x = r.drawTextRun(dst, f, x, y)
			}
		}
	}
}

// drawTextRun shapes and draws the run of text cells starting at (x0, y)
// that share a face, returning the column after the run. Ligature-capable
// fonts in the pack substitute glyphs while keeping one glyph per cell,
// so each glyph anchors to its cluster's origin cell.
func (r *Rasterizer) drawTextRun(dst *image.RGBA, f *vtengine.Frame, x0, y int) int {
	runFace := r.pickCellFace(f.CellAt(x0, y))

	var runes []rune
	var originCell []int
	x := x0
	for x < f.Geometry.Cols {
		c := f.CellAt(x, y)
		if len(c.Runes) == 0 || spriteCell(c) || r.isEmojiCell(c.Runes) {
			break
		}
		if r.pickCellFace(c) != runFace {
			break
		}
		for _, rn := range c.Runes {
			runes = append(runes, rn)
			originCell = append(originCell, x)
		}
		x += max(int(c.Width), 1)
	}

	out := r.shape(runFace, runes)
	baselineY := r.orgY + y*r.cellH + r.baseline
	for _, g := range out.Glyphs {
		cellX := originCell[g.TextIndex()]
		st := f.CellAt(cellX, y).Style
		bg, fg := effectiveColors(st, f)
		if st.Faint {
			fg = mix(fg, bg)
		}
		mask := r.mask(runFace, g.GlyphID)
		if mask == nil {
			continue
		}
		penX := r.orgX + cellX*r.cellW + g.XOffset.Round()
		blitMask(dst, mask, penX, baselineY-g.YOffset.Round(), fg)
	}

	// Decorations per cell, over the glyphs.
	for cx := x0; cx < x; cx++ {
		r.drawDecorations(dst, f, cx, y)
	}
	return x
}

// mask renders (and caches) the alpha mask of a glyph outline.
func (r *Rasterizer) mask(face *font.Face, gid font.GID) *glyphMask {
	key := glyphKey{face: face, gid: gid}
	if m, ok := r.glyphs[key]; ok {
		return m
	}
	m := r.maskAt(face, gid, r.sizePx)
	r.glyphs[key] = m
	return m
}

// maskAt renders a glyph mask at an explicit pixel size, uncached — the
// title strip renders once per recording and caches ITSELF.
func (r *Rasterizer) maskAt(face *font.Face, gid font.GID, sizePx int) *glyphMask {
	outline, ok := face.GlyphData(gid).(font.GlyphOutline)
	if !ok || len(outline.Segments) == 0 {
		return nil
	}
	scale := float32(sizePx) / float32(face.Upem())

	minX, minY := float32(1e9), float32(1e9)
	maxX, maxY := float32(-1e9), float32(-1e9)
	visit := func(px, py float32) {
		x, yy := px*scale, py*scale
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, yy), max(maxY, yy)
	}
	for _, seg := range outline.Segments {
		for i := 0; i < segPointCount(seg.Op); i++ {
			p := seg.Args[i]
			visit(p.X, p.Y)
		}
	}

	left := int(minX) - 1
	top := int(maxY) + 2
	w := int(maxX) - left + 2
	h := top - (int(minY) - 2)
	if w <= 0 || h <= 0 {
		return nil
	}

	ras := vector.NewRasterizer(w, h)
	ox, oy := float32(-left), float32(top)
	// The explicit float32 conversions round the product BEFORE the add,
	// which forbids the compiler from fusing `c + a*b` into a single FMA
	// (Go spec: fusion must not discard an explicit rounding). arm64 fuses,
	// amd64 does not — without the barrier the coverage of one glyph edge
	// can differ by one alpha step and break cross-arch golden equality.
	sx := func(p ot.SegmentPoint) float32 { return ox + float32(p.X*scale) }
	sy := func(p ot.SegmentPoint) float32 { return oy - float32(p.Y*scale) }
	for _, seg := range outline.Segments {
		p := seg.Args
		switch seg.Op {
		case ot.SegmentOpMoveTo:
			ras.MoveTo(sx(p[0]), sy(p[0]))
		case ot.SegmentOpLineTo:
			ras.LineTo(sx(p[0]), sy(p[0]))
		case ot.SegmentOpQuadTo:
			ras.QuadTo(sx(p[0]), sy(p[0]), sx(p[1]), sy(p[1]))
		case ot.SegmentOpCubeTo:
			ras.CubeTo(sx(p[0]), sy(p[0]), sx(p[1]), sy(p[1]), sx(p[2]), sy(p[2]))
		}
	}
	alpha := image.NewAlpha(image.Rect(0, 0, w, h))
	ras.Draw(alpha, alpha.Bounds(), image.Opaque, image.Point{})

	return &glyphMask{alpha: alpha, left: left, top: top}
}

func segPointCount(op ot.SegmentOp) int {
	switch op {
	case ot.SegmentOpMoveTo, ot.SegmentOpLineTo:
		return 1
	case ot.SegmentOpQuadTo:
		return 2
	case ot.SegmentOpCubeTo:
		return 3
	default:
		return 0
	}
}

// blitMask draws an alpha mask with the given color at (penX, baselineY).
func blitMask(dst *image.RGBA, m *glyphMask, penX, baselineY int, fg color.RGBA) {
	b := m.alpha.Bounds()
	for j := 0; j < b.Dy(); j++ {
		dy := baselineY - m.top + j
		if dy < 0 || dy >= dst.Bounds().Dy() {
			continue
		}
		for i := 0; i < b.Dx(); i++ {
			a := m.alpha.Pix[j*m.alpha.Stride+i]
			if a == 0 {
				continue
			}
			dx := penX + m.left + i
			if dx < 0 || dx >= dst.Bounds().Dx() {
				continue
			}
			o := dst.PixOffset(dx, dy)
			ia := 255 - int(a)
			// (fg*a + dst*(255-a))/255 <= 255 by construction.
			dst.Pix[o] = uint8((int(fg.R)*int(a) + int(dst.Pix[o])*ia) / 255)     //nolint:gosec
			dst.Pix[o+1] = uint8((int(fg.G)*int(a) + int(dst.Pix[o+1])*ia) / 255) //nolint:gosec
			dst.Pix[o+2] = uint8((int(fg.B)*int(a) + int(dst.Pix[o+2])*ia) / 255) //nolint:gosec
			dst.Pix[o+3] = 0xff
		}
	}
}

func mix(a, b color.RGBA) color.RGBA {
	// (x + y) / 2 <= 255 for uint8 inputs by construction.
	return color.RGBA{
		R: uint8((int(a.R) + int(b.R)) / 2), //nolint:gosec
		G: uint8((int(a.G) + int(b.G)) / 2), //nolint:gosec
		B: uint8((int(a.B) + int(b.B)) / 2), //nolint:gosec
		A: 0xff,
	}
}
