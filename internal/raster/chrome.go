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

// The window bar styles: VHS's four, plus foley's genre controls
// (dress-reachable; a tape using them still degrades in VHS, which
// silently draws no bar for unknown styles).
const (
	BarNone          BarStyle = iota
	BarColorful               // macOS traffic lights, left
	BarColorfulRight          // macOS traffic lights, right
	BarRings
	BarRingsRight
	BarLinuxControls // minimize/maximize/close strokes, right
	BarGnomeCSD      // a close button in a circle, right (CSD-like)
)

// TitleAlign positions the window title inside the bar.
type TitleAlign uint8

// Title alignments.
const (
	TitleCenter TitleAlign = iota
	TitleLeft
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
	// Title is drawn inside the bar (static text: recordings must not
	// leak hostnames — determinism). TitleAlign positions it.
	Title      string
	TitleAlign TitleAlign
	// KeysBand is the height of the input-caption band under the
	// window (ADR-016); zero = no band. The canvas GROWS by it — a cue
	// never eats grid rows and the footage is never covered.
	KeysBand int
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
func (r *Rasterizer) drawChrome(dst *image.RGBA, bg, fg color.RGBA) {
	r.titleFG = fg
	w := r.opts.Window
	if !w.enabled() {
		return
	}
	s := r.s
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
	// With a keys band the block STOPS above it — the caps float on
	// the margin fill under the window, like the mockup's stage; the
	// caps themselves draw per frame (they animate), not here.
	kb := w.KeysBand * s
	winTop := m
	if bh := w.barHeight() * s; bh > 0 {
		r.drawBar(dst, image.Rect(m, winTop, cw-m, winTop+bh), bg)
		winTop += bh
	}
	fillRect(dst, image.Rect(m, winTop, cw-m, ch-kb-m), bg)
	if kb > 0 {
		// The input reel (ADR-016 v3): the band is the MARGIN running
		// under the window — the dress is the desk, the theme the film
		// (the stage died: three near-blacks read as mud) — with the
		// strip square edge to edge and its perforations PUNCHED
		// through to the fill behind it: a hole shows what is behind,
		// never an invented gray. Step 1 already painted the margin
		// across the band. Plain drops the celluloid entirely; the key
		// frames draw per RECORDING frame — this is chrome.
		bandTop := ch - kb
		stripTop := bandTop + keysBandPadTop*s
		stripBot := ch - keysBandPadBot*s
		r.bandRect = image.Rect(m, stripTop, cw-m, stripBot)
		if !r.keysStyle.Plain {
			fillRect(dst, image.Rect(0, stripTop, cw, stripBot), filmShade(bg))
			hw, hh := keysSprocketW*s, keysSprocketH*s
			pitch := (keysSprocketW + keysSprocketGap) * s
			topY := stripTop + keysSprocketPad*s
			botY := stripBot - keysSprocketPad*s - hh
			for x := pitch / 2; x+hw <= cw; x += pitch {
				r.punchHole(dst, image.Rect(x, topY, x+hw, topY+hh))
				r.punchHole(dst, image.Rect(x, botY, x+hw, botY+hh))
			}
		}
	}
}

// punchHole paints one sprocket perforation with the margin fill
// behind the strip — the fill's color, or the image fill's own pixels
// (the same cached buffer the corner reveals sample). A hole is a
// hole (ADR-016 v3).
func (r *Rasterizer) punchHole(dst *image.RGBA, rect image.Rectangle) {
	rad := keysSprocketRad * r.s
	if r.marginBuf == nil {
		fillRoundedRect(dst, rect, rad, r.opts.Window.MarginFill.Color, 255)
		return
	}
	fillRoundedRectAt(dst, rect, rad, func(x, y int) color.RGBA { return r.marginBuf.RGBAAt(x, y) })
}

// barShade derives a bar background that READS as a bar over any theme:
// lighten dark backgrounds, darken light ones — a bar that inherits the
// content background is invisible (dots floating in space).
func barShade(bg color.RGBA) color.RGBA {
	lum := (299*int(bg.R) + 587*int(bg.G) + 114*int(bg.B)) / 1000
	f := func(c uint8) uint8 {
		if lum < 128 {
			return uint8(int(c) + (255-int(c))*12/100) //nolint:gosec // bounded
		}
		return uint8(int(c) * 90 / 100) //nolint:gosec // bounded
	}
	return color.RGBA{R: f(bg.R), G: f(bg.G), B: f(bg.B), A: 0xff}
}

// mixRGBA blends a toward b by pct (integer math, deterministic).
func mixRGBA(a, b color.RGBA, pct int) color.RGBA {
	m := func(x, y uint8) uint8 {
		return uint8((int(x)*(100-pct) + int(y)*pct) / 100) //nolint:gosec // bounded
	}
	return color.RGBA{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: 0xff}
}

// resolvedBarColor returns the explicit bar color, or the theme-derived
// shade when unset (zero value).
func (r *Rasterizer) resolvedBarColor(contentBG color.RGBA) color.RGBA {
	if r.opts.Window.BarColor.A != 0 {
		return r.opts.Window.BarColor
	}
	return barShade(contentBG)
}

// drawBar renders the bar styles into rect (already scaled): background
// shade, style controls, title, and the divider line every real
// terminal draws under its bar.
func (r *Rasterizer) drawBar(dst *image.RGBA, rect image.Rectangle, contentBG color.RGBA) {
	w := r.opts.Window
	s := r.s
	barBG := r.resolvedBarColor(contentBG)
	fillRect(dst, rect, barBG)
	// Divider: a subtle dark edge between bar and content.
	div := mixRGBA(barBG, color.RGBA{A: 0xff}, 35)
	fillRect(dst, image.Rect(rect.Min.X, rect.Max.Y-s, rect.Max.X, rect.Max.Y), div)
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
	case BarLinuxControls:
		// Three stroke glyphs on the right: minus, hollow square, X —
		// the linux/window-manager genre from the dress spec shots.
		side := barSize / 3
		gap := (barSize - side) / 2
		space := side + barSize/4
		stroke := max(s, barSize/14)
		c := mixRGBA(barBG, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, 72)
		cy := rect.Min.Y + barSize/2
		for i := 0; i < 3; i++ {
			cx := rect.Max.X - (gap + side/2 + (2-i)*space)
			switch i {
			case 0: // minimize
				fillRect(dst, image.Rect(cx-side/2, cy-stroke/2, cx+side/2, cy-stroke/2+stroke), c)
			case 1: // maximize (hollow square)
				x0, y0, x1, y1 := cx-side/2, cy-side/2, cx+side/2, cy+side/2
				fillRect(dst, image.Rect(x0, y0, x1, y0+stroke), c)
				fillRect(dst, image.Rect(x0, y1-stroke, x1, y1), c)
				fillRect(dst, image.Rect(x0, y0, x0+stroke, y1), c)
				fillRect(dst, image.Rect(x1-stroke, y0, x1, y1), c)
			case 2: // close (X)
				a := image.NewAlpha(image.Rect(0, 0, side, side))
				fs := float64(side)
				strokeAlpha(a, [][]spritePt{{{0, 0}, {fs, fs}}, {{fs, 0}, {0, fs}}}, float64(stroke))
				blitMask(dst, &glyphMask{alpha: a}, cx-side/2, cy-side/2, c)
			}
		}
	case BarGnomeCSD:
		// One CSD-like close button: a small X in a subtle circle,
		// right, GNOME headerbar proportions.
		rad := barSize / 4
		cx := rect.Max.X - (barSize/2 + rad)
		cy := rect.Min.Y + barSize/2
		fillCircle(dst, cx, cy, rad, mixRGBA(barBG, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, 14))
		xr := (rad * 11) / 20
		a := image.NewAlpha(image.Rect(0, 0, 2*xr, 2*xr))
		fs := float64(2 * xr)
		strokeAlpha(a, [][]spritePt{{{0, 0}, {fs, fs}}, {{fs, 0}, {0, fs}}}, float64(max(s, barSize/20)))
		blitMask(dst, &glyphMask{alpha: a}, cx-xr, cy-xr, mixRGBA(barBG, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, 80))
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
			fillCircle(dst, cx, cy, innerRad, barBG)
		}
	}

	r.drawBarTitle(dst, rect, barBG)
}

// drawBarTitle blits the window title into the bar. The rendered strip
// is cached: title, bar and metrics are fixed for a whole recording.
func (r *Rasterizer) drawBarTitle(dst *image.RGBA, rect image.Rectangle, barBG color.RGBA) {
	w := r.opts.Window
	if w.Title == "" {
		return
	}
	if r.titleMask == nil {
		r.titleMask = r.renderTitleStrip(w.Title, w.BarSize*r.s)
	}
	if r.titleMask == nil {
		return
	}
	b := r.titleMask.alpha.Bounds()
	barSize := w.BarSize * r.s
	// Keep clear of the controls: one bar-height is enough when they sit
	// right (or there are none), but LEFT controls span 5/3·barSize (VHS
	// dot geometry: gap ⅓ + radius ⅙ + two ½ spacings + radius ⅙), so a
	// left title there insets two bar-heights.
	x := rect.Min.X + barSize
	if w.Bar == BarColorful || w.Bar == BarRings {
		x = rect.Min.X + 2*barSize
	}
	if w.TitleAlign == TitleCenter {
		x = rect.Min.X + (rect.Dx()-b.Dx())/2
	}
	y := rect.Min.Y + (barSize-b.Dy())/2
	blitMask(dst, r.titleMask, x, y, mixRGBA(barBG, r.titleFG, 62))
}

// renderTitleStrip shapes and rasters the title once, at a size derived
// from the bar (55% of its height), into an alpha strip.
func (r *Rasterizer) renderTitleStrip(title string, barPx int) *glyphMask {
	m, _ := r.renderTextStrip(title, max(8*r.s, (barPx*11)/20))
	return m
}

// renderTextStrip shapes and rasters a text run at an explicit pixel
// size into an alpha strip — the bar title and the key frames share
// it. The returned ascent lets callers align BASELINES across strips
// of different sizes (a cap's counter next to its key).
func (r *Rasterizer) renderTextStrip(title string, px int) (*glyphMask, int) {
	face := r.gridFace() // a user font labels its own window
	out := r.shapeAt(face, []rune(title), px)
	asc := out.LineBounds.Ascent.Round()
	desc := -out.LineBounds.Descent.Round()
	if asc <= 0 {
		return nil, 0
	}
	width := 0
	for _, g := range out.Glyphs {
		width += g.Advance.Round()
	}
	if width <= 0 {
		return nil, 0
	}
	strip := image.NewAlpha(image.Rect(0, 0, width, asc+desc))
	pen := 0
	for _, g := range out.Glyphs {
		if m := r.maskAt(face, g.GlyphID, px); m != nil {
			blitAlpha(strip, m, pen+g.XOffset.Round(), asc-g.YOffset.Round())
		}
		pen += g.Advance.Round()
	}
	return &glyphMask{alpha: strip}, asc
}

// blitAlpha maxes a glyph mask into an alpha strip at (penX, baselineY).
func blitAlpha(dst *image.Alpha, m *glyphMask, penX, baselineY int) {
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
			o := dy*dst.Stride + dx
			if dst.Pix[o] < a {
				dst.Pix[o] = a
			}
		}
	}
}

// fillRoundedRect blends a rounded rectangle over dst at the given
// opacity (0-255) — the key chips' body. Corner coverage reuses the
// half-pixel AA circle.
func fillRoundedRect(dst *image.RGBA, rect image.Rectangle, rad int, c color.RGBA, alpha int) {
	if alpha <= 0 {
		return
	}
	b := dst.Bounds()
	for y := max(rect.Min.Y, 0); y < min(rect.Max.Y, b.Max.Y); y++ {
		for x := max(rect.Min.X, 0); x < min(rect.Max.X, b.Max.X); x++ {
			cov := 1.0
			left, right := x < rect.Min.X+rad, x >= rect.Max.X-rad
			top, bot := y < rect.Min.Y+rad, y >= rect.Max.Y-rad
			if (left || right) && (top || bot) {
				cx, cy := rect.Min.X+rad, rect.Min.Y+rad
				if right {
					cx = rect.Max.X - rad
				}
				if bot {
					cy = rect.Max.Y - rad
				}
				cov = circleCoverage(x, y, cx, cy, rad+1)
			}
			a := int(math.Round(cov*255)) * alpha / 255
			if a <= 0 {
				continue
			}
			o := dst.PixOffset(x, y)
			ia := 255 - a
			dst.Pix[o] = uint8((int(c.R)*a + int(dst.Pix[o])*ia) / 255)     //nolint:gosec // bounded by construction
			dst.Pix[o+1] = uint8((int(c.G)*a + int(dst.Pix[o+1])*ia) / 255) //nolint:gosec
			dst.Pix[o+2] = uint8((int(c.B)*a + int(dst.Pix[o+2])*ia) / 255) //nolint:gosec
			dst.Pix[o+3] = 0xff
		}
	}
}

// fillRoundedRectAt is fillRoundedRect with a per-pixel color source —
// punching sprocket holes through to an image margin fill. Opaque:
// what shows through a hole is the fill itself, not a blend of it.
func fillRoundedRectAt(dst *image.RGBA, rect image.Rectangle, rad int, at func(x, y int) color.RGBA) {
	b := dst.Bounds()
	for y := max(rect.Min.Y, 0); y < min(rect.Max.Y, b.Max.Y); y++ {
		for x := max(rect.Min.X, 0); x < min(rect.Max.X, b.Max.X); x++ {
			cov := 1.0
			left, right := x < rect.Min.X+rad, x >= rect.Max.X-rad
			top, bot := y < rect.Min.Y+rad, y >= rect.Max.Y-rad
			if (left || right) && (top || bot) {
				cx, cy := rect.Min.X+rad, rect.Min.Y+rad
				if right {
					cx = rect.Max.X - rad
				}
				if bot {
					cy = rect.Max.Y - rad
				}
				cov = circleCoverage(x, y, cx, cy, rad+1)
			}
			a := int(math.Round(cov * 255))
			if a <= 0 {
				continue
			}
			c := at(x, y)
			o := dst.PixOffset(x, y)
			ia := 255 - a
			dst.Pix[o] = uint8((int(c.R)*a + int(dst.Pix[o])*ia) / 255)     //nolint:gosec // bounded by construction
			dst.Pix[o+1] = uint8((int(c.G)*a + int(dst.Pix[o+1])*ia) / 255) //nolint:gosec
			dst.Pix[o+2] = uint8((int(c.B)*a + int(dst.Pix[o+2])*ia) / 255) //nolint:gosec
			dst.Pix[o+3] = 0xff
		}
	}
}

// blitMaskFaded is blitMask with an extra opacity multiplier — fading
// chip labels.
func blitMaskFaded(dst *image.RGBA, m *glyphMask, x, y int, c color.RGBA, alpha int) {
	if alpha >= 255 {
		blitMask(dst, m, x, y, c)
		return
	}
	if alpha <= 0 {
		return
	}
	mb := m.alpha.Bounds()
	db := dst.Bounds()
	for j := 0; j < mb.Dy(); j++ {
		dy := y + j
		if dy < 0 || dy >= db.Max.Y {
			continue
		}
		for i := 0; i < mb.Dx(); i++ {
			a := int(m.alpha.Pix[j*m.alpha.Stride+i]) * alpha / 255
			if a == 0 {
				continue
			}
			dx := x + i
			if dx < 0 || dx >= db.Max.X {
				continue
			}
			o := dst.PixOffset(dx, dy)
			ia := 255 - a
			dst.Pix[o] = uint8((int(c.R)*a + int(dst.Pix[o])*ia) / 255)     //nolint:gosec // bounded by construction
			dst.Pix[o+1] = uint8((int(c.G)*a + int(dst.Pix[o+1])*ia) / 255) //nolint:gosec
			dst.Pix[o+2] = uint8((int(c.B)*a + int(dst.Pix[o+2])*ia) / 255) //nolint:gosec
			dst.Pix[o+3] = 0xff
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
	s := r.s
	rad := radius * s
	m := w.Margin * s
	cw, ch := w.CanvasW*s, w.CanvasH*s
	// The keys band sits below the window block: corners round the
	// BLOCK, not the canvas.
	ch -= w.KeysBand * s
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
		// The block's bottom corners reveal the margin too: with the
		// reel below, the margin RUNS under the window into the band
		// (ADR-016 v3 — the stage died).
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
