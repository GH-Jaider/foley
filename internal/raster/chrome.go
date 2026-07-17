package raster

import (
	"image"
	"image/color"
	"math"

	xdraw "golang.org/x/image/draw"
)

// Window chrome (VHS parity): the tape's Width/Height are the FINAL
// canvas, and margin, window bar and padding all eat space from the
// terminal grid — exactly VHS's arithmetic (ffmpeg.go of the pinned
// release). Geometry and drawing replicate VHS's draw.go (MIT): dot
// radii and colors, ring bars, and the half-pixel antialiased circle of
// the corner mask. All values here are LOGICAL pixels; the rasterizer
// multiplies by Scale.
//
// Composition order per frame: margin fill (color or scaled image) →
// window block (bar + terminal background) → grid content at its origin
// → rounded corners re-reveal the margin fill (mask applied last, over
// everything, like VHS masks the whole window block).

// BarStyle is a WindowBar variant. The zero value means no bar.
type BarStyle uint8

// The VHS window bar styles.
const (
	BarNone BarStyle = iota
	BarColorful
	BarColorfulRight
	BarRings
	BarRingsRight
)

// Fill is a margin fill: a solid color, or an image file already decoded
// by the caller (scaled to the canvas here). Image wins when non-nil.
type Fill struct {
	Color color.RGBA
	Image image.Image
}

// Window configures the chrome. Zero value = no chrome at all: the
// canvas is exactly the grid (every pre-chrome recording and golden).
type Window struct {
	// CanvasW, CanvasH are the final output size in logical pixels.
	// Zero means "grid only" (no chrome).
	CanvasW, CanvasH int
	// Padding is the inner border between the terminal background and
	// the grid, painted in the theme background.
	Padding int
	// Margin is the border outside the window block, painted MarginFill.
	Margin int
	// MarginFill paints the margin band (and the corner reveals).
	MarginFill Fill
	// Bar selects the window bar style; BarSize its height; BarColor its
	// background.
	Bar      BarStyle
	BarSize  int
	BarColor color.RGBA
	// Radius rounds the window block's corners, revealing MarginFill.
	Radius int
}

func (w Window) enabled() bool { return w.CanvasW > 0 && w.CanvasH > 0 }

// barHeight is the bar's logical height, zero when there is no bar.
func (w Window) barHeight() int {
	if w.Bar == BarNone {
		return 0
	}
	return w.BarSize
}

// contentOrigin is where the GRID starts on the canvas, logical px.
func (w Window) contentOrigin() (x, y int) {
	if !w.enabled() {
		return 0, 0
	}
	return w.Margin + w.Padding, w.Margin + w.barHeight() + w.Padding
}

// drawChrome paints everything around the grid. dst is the full scaled
// canvas; the grid pipeline draws after this, and roundCorners after
// everything.
func (r *Rasterizer) drawChrome(dst *image.RGBA, bg color.RGBA) {
	w := r.opts.Window
	if !w.enabled() {
		return
	}
	s := r.opts.Scale
	cw, ch := w.CanvasW*s, w.CanvasH*s
	m := w.Margin * s

	// 1. Margin fill under everything. An image fill is scaled ONCE into
	// a cached buffer: frames reuse it, and the corner reveals sample it.
	if w.MarginFill.Image != nil {
		if r.marginBuf == nil || r.marginBuf.Bounds().Dx() != cw || r.marginBuf.Bounds().Dy() != ch {
			r.marginBuf = image.NewRGBA(image.Rect(0, 0, cw, ch))
			xdraw.ApproxBiLinear.Scale(r.marginBuf, r.marginBuf.Bounds(),
				w.MarginFill.Image, w.MarginFill.Image.Bounds(), xdraw.Src, nil)
		}
		copy(dst.Pix, r.marginBuf.Pix)
	} else {
		fillRect(dst, image.Rect(0, 0, cw, ch), w.MarginFill.Color)
	}

	// 2. Window block: bar on top, terminal background below (the
	// padding border is simply background the grid does not cover).
	winTop := m
	if bh := w.barHeight() * s; bh > 0 {
		r.drawBar(dst, image.Rect(m, winTop, cw-m, winTop+bh))
		winTop += bh
	}
	fillRect(dst, image.Rect(m, winTop, cw-m, ch-m), bg)
}

// drawBar renders the VHS bar styles into rect (already scaled).
func (r *Rasterizer) drawBar(dst *image.RGBA, rect image.Rectangle) {
	w := r.opts.Window
	s := r.opts.Scale
	fillRect(dst, rect, w.BarColor)
	barSize := w.BarSize * s

	switch w.Bar {
	case BarNone:
	case BarColorful, BarColorfulRight:
		// VHS draw.go: dotRad = barSize/6, gap = (barSize-2*dotRad)/2,
		// space between centers = 2*dotRad + barSize/6.
		dotRad := barSize / 6
		dotGap := (barSize - 2*dotRad) / 2
		dotSpace := 2*dotRad + barSize/6
		colors := [3]color.RGBA{
			{R: 0xFF, G: 0x4F, B: 0x4D, A: 0xFF},
			{R: 0xFE, G: 0xBB, B: 0x00, A: 0xFF},
			{R: 0x00, G: 0xCC, B: 0x1D, A: 0xFF},
		}
		cy := rect.Min.Y + dotRad + dotGap
		for i, c := range colors {
			cx := rect.Min.X + dotGap + dotRad + i*dotSpace
			if w.Bar == BarColorfulRight {
				cx = rect.Max.X - (dotGap + dotRad + i*dotSpace)
			}
			fillCircle(dst, cx, cy, dotRad, c)
		}
	case BarRings, BarRingsRight:
		// VHS: outerRad = barSize/5, innerRad = 2*(2*outerRad)/5,
		// gap = (barSize-2*outerRad)/2, space = 2*outerRad + barSize/6.
		outerRad := barSize / 5
		innerRad := 2 * (2 * outerRad) / 5
		ringGap := (barSize - 2*outerRad) / 2
		ringSpace := 2*outerRad + barSize/6
		ring := color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xFF}
		cy := rect.Min.Y + outerRad + ringGap
		for i := 0; i <= 2; i++ {
			cx := rect.Min.X + ringGap + outerRad + i*ringSpace
			if w.Bar == BarRingsRight {
				cx = rect.Max.X - (ringGap + outerRad + i*ringSpace)
			}
			fillCircle(dst, cx, cy, outerRad, ring)
			fillCircle(dst, cx, cy, innerRad, w.BarColor)
		}
	}
}

// circleCoverage is VHS's antialiased circle (draw.go At), verbatim: the
// distance from the pixel (half-pixel centered) to a circle of radius
// r-1; inside → 1, within 1px → linear falloff. The float64 products go
// through explicit conversions — the package FMA barrier rule.
func circleCoverage(x, y, cx, cy, rad int) float64 {
	xx := float64(x-cx) + 0.5
	yy := float64(y-cy) + 0.5
	rr := float64(rad) - 1
	dist := math.Sqrt(float64(xx*xx)+float64(yy*yy)) - rr
	switch {
	case dist < 0:
		return 1
	case dist <= 1:
		return 1 - dist
	default:
		return 0
	}
}

// fillCircle blends an antialiased solid circle over dst.
func fillCircle(dst *image.RGBA, cx, cy, rad int, c color.RGBA) {
	b := dst.Bounds()
	for y := max(cy-rad, 0); y < min(cy+rad, b.Max.Y); y++ {
		for x := max(cx-rad, 0); x < min(cx+rad, b.Max.X); x++ {
			cov := circleCoverage(x, y, cx, cy, rad)
			if cov <= 0 {
				continue
			}
			a := int(math.Round(cov * 255))
			o := dst.PixOffset(x, y)
			ia := 255 - a
			dst.Pix[o] = uint8((int(c.R)*a + int(dst.Pix[o])*ia) / 255)     //nolint:gosec // bounded by construction
			dst.Pix[o+1] = uint8((int(c.G)*a + int(dst.Pix[o+1])*ia) / 255) //nolint:gosec
			dst.Pix[o+2] = uint8((int(c.B)*a + int(dst.Pix[o+2])*ia) / 255) //nolint:gosec
			dst.Pix[o+3] = 0xff
		}
	}
}

// roundCorners re-reveals the margin fill on the window block's corners,
// after everything else has drawn — VHS applies its mask to the whole
// block (bar + terminal) the same way. radius is logical px.
func (r *Rasterizer) roundCorners(dst *image.RGBA) {
	w := r.opts.Window
	radius := w.Radius
	if !w.enabled() || radius <= 0 {
		return
	}
	s := r.opts.Scale
	rad := radius * s
	m := w.Margin * s
	cw, ch := w.CanvasW*s, w.CanvasH*s
	// Window block corners (inside the margin).
	corners := [4]struct{ x, y, cx, cy int }{
		{m, m, m + rad, m + rad},                                 // top-left
		{cw - m - rad, m, cw - m - rad, m + rad},                 // top-right
		{m, ch - m - rad, m + rad, ch - m - rad},                 // bottom-left
		{cw - m - rad, ch - m - rad, cw - m - rad, ch - m - rad}, // bottom-right
	}
	marginAt := func(x, y int) color.RGBA {
		if w.MarginFill.Image == nil {
			return w.MarginFill.Color
		}
		// Sample the cached canvas-scaled fill drawChrome built.
		return r.marginBuf.RGBAAt(x, y)
	}
	for _, c := range corners {
		for y := c.y; y < c.y+rad; y++ {
			for x := c.x; x < c.x+rad; x++ {
				if x < 0 || y < 0 || x >= cw || y >= ch {
					continue
				}
				// VHS: mask = roundedrect with circles of radius+1.
				cov := circleCoverage(x, y, c.cx, c.cy, rad+1)
				if cov >= 1 {
					continue // fully inside the window: untouched
				}
				fill := marginAt(x, y)
				a := int(math.Round((1 - cov) * 255))
				o := dst.PixOffset(x, y)
				ia := 255 - a
				dst.Pix[o] = uint8((int(fill.R)*a + int(dst.Pix[o])*ia) / 255)     //nolint:gosec // bounded by construction
				dst.Pix[o+1] = uint8((int(fill.G)*a + int(dst.Pix[o+1])*ia) / 255) //nolint:gosec
				dst.Pix[o+2] = uint8((int(fill.B)*a + int(dst.Pix[o+2])*ia) / 255) //nolint:gosec
				dst.Pix[o+3] = 0xff
			}
		}
	}
}
