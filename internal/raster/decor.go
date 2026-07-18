package raster

import (
	"image"
	"image/color"
	"math"

	"github.com/GH-Jaider/foley/internal/vtengine"
)

// drawDecorations paints underline variants, strikethrough and overline
// for one cell, using the cell's effective foreground.
func (r *Rasterizer) drawDecorations(dst *image.RGBA, f *vtengine.Frame, x, y int) {
	st := f.CellAt(x, y).Style
	if st.Underline == vtengine.UnderlineNone && !st.Strikethrough && !st.Overline {
		return
	}
	_, fg := effectiveColors(st, f)
	cell := r.cellRect(x, y, 1)
	x0, x1 := cell.Min.X, cell.Max.X
	th := r.thickness

	// Every decoration clips to its own cell: a curly underline's sine
	// must never bleed into the next row.
	fill := func(rect image.Rectangle, c color.RGBA) {
		fillRect(dst, rect.Intersect(cell), c)
	}
	hline := func(yy int) {
		fill(image.Rect(x0, yy, x1, yy+th), fg)
	}

	uy := r.orgY + y*r.cellH + r.underline
	// The contract guarantees UnderlineColor is resolved (== FG when the
	// app did not set one), so it is used unconditionally — except under
	// inverse video, where decorations follow the effective foreground.
	ulColor := rgba(st.UnderlineColor)
	if st.Inverse {
		ulColor = fg
	}
	ul := func(yy int) { fill(image.Rect(x0, yy, x1, yy+th), ulColor) }

	switch st.Underline {
	case vtengine.UnderlineNone:
	case vtengine.UnderlineSingle:
		ul(uy)
	case vtengine.UnderlineDouble:
		ul(uy - th)
		ul(uy + th)
	case vtengine.UnderlineDotted:
		for xx := x0; xx < x1; xx += 2 * th {
			fill(image.Rect(xx, uy, min(xx+th, x1), uy+th), ulColor)
		}
	case vtengine.UnderlineDashed:
		dash := r.cellW / 3
		fill(image.Rect(x0, uy, x0+dash, uy+th), ulColor)
		fill(image.Rect(x1-dash, uy, x1, uy+th), ulColor)
	case vtengine.UnderlineCurly:
		amp := float64(max(th, 2*r.s))
		for xx := x0; xx < x1; xx++ {
			phase := 2 * math.Pi * float64(xx-x0) / float64(r.cellW)
			yy := uy + int(math.Round(amp*math.Sin(phase)))
			fill(image.Rect(xx, yy, xx+1, yy+th), ulColor)
		}
	}

	if st.Strikethrough {
		hline(r.orgY + y*r.cellH + r.baseline - r.sizePx/3)
	}
	if st.Overline {
		hline(r.orgY + y*r.cellH + th)
	}
}

func (r *Rasterizer) drawCursor(dst *image.RGBA, f *vtengine.Frame) {
	if !f.Cursor.Visible {
		return
	}
	rect := r.cellRect(f.Cursor.X, f.Cursor.Y, 1)
	fg := rgba(f.Colors.Cursor)
	th := max(1, 2*r.s)
	switch f.Cursor.Shape {
	case vtengine.CursorBlock:
		// v1: solid block over the glyph (glyph inversion under the block
		// is a polish item tracked for the golden suite).
		fillRect(dst, rect, fg)
	case vtengine.CursorBar:
		fillRect(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+th, rect.Max.Y), fg)
	case vtengine.CursorUnderline:
		fillRect(dst, image.Rect(rect.Min.X, rect.Max.Y-th, rect.Max.X, rect.Max.Y), fg)
	case vtengine.CursorHollowBlock:
		s := max(1, r.s)
		fillRect(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+s), fg)
		fillRect(dst, image.Rect(rect.Min.X, rect.Max.Y-s, rect.Max.X, rect.Max.Y), fg)
		fillRect(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+s, rect.Max.Y), fg)
		fillRect(dst, image.Rect(rect.Max.X-s, rect.Min.Y, rect.Max.X, rect.Max.Y), fg)
	}
}
