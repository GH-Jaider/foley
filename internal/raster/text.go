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

func (r *Rasterizer) computeMetrics() {
	out := r.shape(r.text, []rune{'M'})
	r.cellW = out.Glyphs[0].Advance.Round()
	asc := out.LineBounds.Ascent.Round()
	desc := -out.LineBounds.Descent.Round() // Descent is negative-down in typesetting
	gap := out.LineBounds.Gap.Round()
	r.cellH = asc + desc + gap
	r.baseline = asc + gap/2
	r.underline = r.baseline + maxi(2*r.opts.Scale, desc/2)
	r.thickness = maxi(r.opts.Scale, r.sizePx/16)
}

func (r *Rasterizer) shape(face *font.Face, runes []rune) shaping.Output {
	return r.shaper.Shape(shaping.Input{
		Text: runes, RunStart: 0, RunEnd: len(runes),
		Face: face, Size: fixed.I(r.sizePx),
		Script: language.Latin, Language: language.NewLanguage("en"),
	})
}

func (r *Rasterizer) pickFace(st vtengine.Style) (*font.Face, faceStyle) {
	switch {
	case st.Bold:
		return r.bold, faceBold
	case st.Italic:
		return r.italic, faceItalic
	default:
		return r.text, faceRegular
	}
}

// isEmojiCell: the text face cannot render the base rune but the emoji
// face can.
func (r *Rasterizer) isEmojiCell(runes []rune) bool {
	if len(runes) == 0 {
		return false
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
			case r.isEmojiCell(cell.Runes):
				r.drawEmojiCell(dst, f, x, y)
				x += maxi(int(cell.Width), 1)
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
	runFace, styleID := r.pickFace(f.CellAt(x0, y).Style)

	var runes []rune
	var originCell []int
	x := x0
	for x < f.Geometry.Cols {
		c := f.CellAt(x, y)
		if len(c.Runes) == 0 || r.isEmojiCell(c.Runes) {
			break
		}
		if fc, _ := r.pickFace(c.Style); fc != runFace {
			break
		}
		for _, rn := range c.Runes {
			runes = append(runes, rn)
			originCell = append(originCell, x)
		}
		x += maxi(int(c.Width), 1)
	}

	out := r.shape(runFace, runes)
	baselineY := y*r.cellH + r.baseline
	for _, g := range out.Glyphs {
		cellX := originCell[g.TextIndex()]
		st := f.CellAt(cellX, y).Style
		bg, fg := effectiveColors(st, f)
		if st.Faint {
			fg = mix(fg, bg)
		}
		mask := r.mask(styleID, runFace, g.GlyphID)
		if mask == nil {
			continue
		}
		penX := cellX*r.cellW + g.XOffset.Round()
		blitMask(dst, mask, penX, baselineY-g.YOffset.Round(), fg)
	}

	// Decorations per cell, over the glyphs.
	for cx := x0; cx < x; cx++ {
		r.drawDecorations(dst, f, cx, y)
	}
	return x
}

// mask renders (and caches) the alpha mask of a glyph outline.
func (r *Rasterizer) mask(style faceStyle, face *font.Face, gid font.GID) *glyphMask {
	key := glyphKey{style: style, gid: gid}
	if m, ok := r.glyphs[key]; ok {
		return m
	}
	outline, ok := face.GlyphData(gid).(font.GlyphOutline)
	if !ok || len(outline.Segments) == 0 {
		r.glyphs[key] = nil
		return nil
	}
	scale := float32(r.sizePx) / float32(face.Upem())

	minX, minY := float32(1e9), float32(1e9)
	maxX, maxY := float32(-1e9), float32(-1e9)
	visit := func(px, py float32) {
		x, yy := px*scale, py*scale
		minX, maxX = minf(minX, x), maxf(maxX, x)
		minY, maxY = minf(minY, yy), maxf(maxY, yy)
	}
	for _, seg := range outline.Segments {
		for i := range segPoints(seg) {
			p := seg.Args[i]
			visit(p.X, p.Y)
		}
	}

	left := int(minX) - 1
	top := int(maxY) + 2
	w := int(maxX) - left + 2
	h := top - (int(minY) - 2)
	if w <= 0 || h <= 0 {
		r.glyphs[key] = nil
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

	m := &glyphMask{alpha: alpha, left: left, top: top}
	r.glyphs[key] = m
	return m
}

func segPoints(seg ot.Segment) []struct{} {
	switch seg.Op {
	case ot.SegmentOpMoveTo, ot.SegmentOpLineTo:
		return make([]struct{}, 1)
	case ot.SegmentOpQuadTo:
		return make([]struct{}, 2)
	case ot.SegmentOpCubeTo:
		return make([]struct{}, 3)
	default:
		return nil
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

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}
