package raster

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/shaping"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/vtengine"
)

// ImageSource resolves kitty-graphics pixels for placements in a frame.
// vtengine.Engine satisfies it; so does the test fake.
type ImageSource interface {
	ImagePixels(id uint32) (vtengine.ImageData, error)
}

// UserFonts is a user-supplied primary font (ADR-015). A single file
// fills Regular and serves every style; a family gives styles their
// own files — absent styles fall back to the Regular face, keeping the
// grid metrics whole. The primary drives cell metrics and titles the
// window bar; the pack stays as per-cell coverage fallback, emoji stay
// Noto, and block sprites stay synthesized.
type UserFonts struct {
	// Label names the font in errors and warnings (a file path or a
	// catalog family name).
	Label                             string
	Regular, Bold, Italic, BoldItalic []byte
}

func (u UserFonts) empty() bool {
	return len(u.Regular) == 0 && len(u.Bold) == 0 &&
		len(u.Italic) == 0 && len(u.BoldItalic) == 0
}

// Options configures a Rasterizer.
type Options struct {
	Pack *fontpack.Pack
	// UserFonts loads a user primary font over the pack (ADR-015).
	UserFonts UserFonts
	// FontSizePx is the glyph size in pixels at Scale 1.
	FontSizePx int
	// Scale multiplies every metric (2 = native supersampling).
	Scale int
	// Window configures the chrome around the grid (VHS parity: margin,
	// window bar, padding, rounded corners). Zero value = no chrome, the
	// canvas is exactly the grid.
	Window Window
	// Keys is the injected input track for the keys band (ADR-016);
	// nil = no chips. Window.KeysBand sizes the band itself.
	Keys *KeysTrack
	// KeysFontPx is the cap label size in logical px (the reel's
	// small/medium/large); zero = FontSizePx.
	KeysFontPx int
}

// Rasterizer turns engine frames into RGBA images. It caches parsed
// faces, glyph masks and decoded images; it is not safe for concurrent
// use (one rasterizer per recording, like the engine).
type Rasterizer struct {
	opts Options

	text, bold, italic, boldItalic *font.Face
	emoji                          *font.Face
	// user holds the primary faces per style slot (styleIdx order:
	// regular, bold, italic, bold-italic); all nil without a user font,
	// and absent family styles alias the user regular face.
	user [4]*font.Face
	// warnings collects assembly findings (e.g. a proportional user
	// font) — the recorder surfaces them; the raster never prints.
	warnings []string

	sizePx    int // FontSizePx * Scale
	cellW     int
	cellH     int
	baseline  int
	underline int // y offset from cell top
	thickness int

	shaper shaping.HarfbuzzShaper // reused: it caches font data internally

	glyphs  map[glyphKey]*glyphMask
	sprites map[rune]*glyphMask
	emojis  map[font.GID]*image.RGBA
	kitty   map[kittyKey]*image.RGBA

	// orgX, orgY offset every grid drawing operation into the window
	// canvas (scaled px). Zero without chrome.
	orgX, orgY int
	// marginBuf caches a canvas-scaled MarginFill image.
	marginBuf *image.RGBA
	// titleMask caches the rendered window-bar title strip; titleFG is
	// the theme foreground the title color derives from.
	titleMask *glyphMask
	titleFG   color.RGBA
	// The keys band (ADR-016): the track feeding the frames, the film
	// strip's rect and the stage shade (set by drawChrome — the block's
	// bottom corners reveal the stage), the cap font size, and the
	// label cache.
	keys      *KeysTrack
	bandRect  image.Rectangle
	stageBG   color.RGBA
	keysCapPx int
	keyStrips map[string]textStrip
	capLabels map[string]string
	keysMult  string
}

// glyphKey caches masks per FACE — the GID space belongs to a face, so
// keying by anything less (a style enum) would blit wrong glyphs the
// moment two faces share a key.
type glyphKey struct {
	face *font.Face
	gid  font.GID
}

type kittyKey struct {
	id  uint32
	gen uint64
}

// New parses the pack's faces and computes the cell metrics that the
// engine geometry must match (Geometry.CellW/CellH = CellSize()).
func New(opts Options) (*Rasterizer, error) {
	if opts.Pack == nil {
		return nil, errors.New("raster: nil font pack")
	}
	if opts.FontSizePx <= 0 || opts.Scale <= 0 {
		return nil, fmt.Errorf("raster: invalid size/scale %d/%d", opts.FontSizePx, opts.Scale)
	}
	r := &Rasterizer{
		opts:      opts,
		sizePx:    opts.FontSizePx * opts.Scale,
		glyphs:    make(map[glyphKey]*glyphMask),
		sprites:   make(map[rune]*glyphMask),
		emojis:    make(map[font.GID]*image.RGBA),
		kitty:     make(map[kittyKey]*image.RGBA),
		keys:      opts.Keys,
		keyStrips: make(map[string]textStrip),
		capLabels: make(map[string]string),
	}
	r.keysCapPx = r.sizePx
	if opts.KeysFontPx > 0 {
		r.keysCapPx = opts.KeysFontPx * opts.Scale
	}
	var err error
	if r.text, err = font.ParseTTF(bytes.NewReader(opts.Pack.Text)); err != nil {
		return nil, fmt.Errorf("raster: text face: %w", err)
	}
	if r.bold, err = font.ParseTTF(bytes.NewReader(opts.Pack.TextBold)); err != nil {
		return nil, fmt.Errorf("raster: bold face: %w", err)
	}
	if r.italic, err = font.ParseTTF(bytes.NewReader(opts.Pack.TextItalic)); err != nil {
		return nil, fmt.Errorf("raster: italic face: %w", err)
	}
	if r.boldItalic, err = font.ParseTTF(bytes.NewReader(opts.Pack.TextBoldItalic)); err != nil {
		return nil, fmt.Errorf("raster: bold-italic face: %w", err)
	}
	if r.emoji, err = font.ParseTTF(bytes.NewReader(opts.Pack.Emoji)); err != nil {
		return nil, fmt.Errorf("raster: emoji face: %w", err)
	}
	if !opts.UserFonts.empty() {
		if len(opts.UserFonts.Regular) == 0 {
			return nil, fmt.Errorf("raster: user font %s: a family needs its regular face (metrics derive from it)", opts.UserFonts.Label)
		}
		parse := func(b []byte, style string) (*font.Face, error) {
			if len(b) == 0 {
				return nil, nil
			}
			f, perr := font.ParseTTF(bytes.NewReader(b))
			if perr != nil {
				return nil, fmt.Errorf("raster: user font %s (%s): %w", opts.UserFonts.Label, style, perr)
			}
			return f, nil
		}
		slots := [4][]byte{opts.UserFonts.Regular, opts.UserFonts.Bold, opts.UserFonts.Italic, opts.UserFonts.BoldItalic}
		for i, b := range slots {
			if r.user[i], err = parse(b, styleNames[i]); err != nil {
				return nil, err
			}
		}
		// Absent family styles alias the regular face: the weight is
		// lost, the metrics are not.
		for i := 1; i < 4; i++ {
			if r.user[i] == nil {
				r.user[i] = r.user[0]
			}
		}
	}
	r.computeMetrics()
	r.checkUserFont()
	// The coalescing counter's multiplication sign, decided once per
	// effective font (× when covered, plain x otherwise).
	r.keysMult = "×"
	if _, ok := r.gridFace().NominalGlyph('×'); !ok {
		r.keysMult = "x"
	}
	if opts.Keys != nil && opts.Window.KeysBand > 0 {
		// Take capacity from what the strip actually fits (a square
		// frame is the minimum): same inputs, same capacity.
		frameH := opts.Window.KeysBand - keysStageTop - keysStageBot - 2*(keysSprocketH+2*keysSprocketPad)
		opts.Keys.setCapacity((opts.Window.CanvasW - 2*opts.Window.Margin) / (frameH + keysCapGap))
	}
	ox, oy := opts.Window.contentOrigin()
	r.orgX, r.orgY = ox*opts.Scale, oy*opts.Scale
	return r, nil
}

// Warnings reports assembly findings for the recorder to surface. The
// slice is a copy — callers cannot corrupt the rasterizer's record.
func (r *Rasterizer) Warnings() []string {
	return append([]string(nil), r.warnings...)
}

// styleNames labels the style slots in warnings, styleIdx order.
var styleNames = [4]string{"regular", "bold", "italic", "bold-italic"} //nolint:gochecknoglobals // immutable label table

// checkUserFont warns — never errors — on user fonts that will not
// look like the user expects (ADR-015): missing basic latin (the grid
// falls back to the pack for metrics and coverage), proportional
// advances, or a family style whose advance disagrees with regular
// (both make a ragged grid — almost always an accident, possibly an
// artistic choice).
func (r *Rasterizer) checkUserFont() {
	if r.user[0] == nil {
		return
	}
	if _, ok := r.user[0].NominalGlyph('M'); !ok {
		r.warnings = append(r.warnings, fmt.Sprintf(
			"user font %s lacks basic latin coverage; grid metrics and uncovered text fall back to the pinned pack",
			r.opts.UserFonts.Label))
		return
	}
	adv := -1
	for _, rn := range []rune{'i', 'l', '1', '.', 'M', 'W', '@'} {
		if _, ok := r.user[0].NominalGlyph(rn); !ok {
			continue
		}
		a := r.shape(r.user[0], []rune{rn}).Glyphs[0].Advance.Round()
		if adv < 0 {
			adv = a
			continue
		}
		if a != adv {
			r.warnings = append(r.warnings, fmt.Sprintf(
				"user font %s is not monospace (advances differ); the grid will look ragged",
				r.opts.UserFonts.Label))
			return
		}
	}
	// A family style with a different 'M' advance breaks the grid the
	// same way (real terminal families keep one advance across styles).
	for i := 1; i < 4; i++ {
		if r.user[i] == r.user[0] {
			continue
		}
		if _, ok := r.user[i].NominalGlyph('M'); !ok {
			continue
		}
		if r.shape(r.user[i], []rune{'M'}).Glyphs[0].Advance.Round() != adv {
			r.warnings = append(r.warnings, fmt.Sprintf(
				"user font %s: the %s face advance differs from regular; the grid will look ragged",
				r.opts.UserFonts.Label, styleNames[i]))
		}
	}
}

// CellSize returns the cell size in output pixels (already scaled).
func (r *Rasterizer) CellSize() (w, h int) { return r.cellW, r.cellH }

// LogicalCellSize returns the cell size in LOGICAL pixels — the values
// the engine geometry and the pty winsize must use (kitty-graphics math
// happens in logical space; Render multiplies by Scale).
func (r *Rasterizer) LogicalCellSize() (w, h int) {
	return r.cellW / r.opts.Scale, r.cellH / r.opts.Scale
}

// Render draws the frame into dst (reused when it has the right bounds,
// reallocated otherwise) and returns the image.
func (r *Rasterizer) Render(f *vtengine.Frame, src ImageSource, dst *image.RGBA) (*image.RGBA, error) {
	w := f.Geometry.Cols * r.cellW
	h := f.Geometry.Rows * r.cellH
	if win := r.opts.Window; win.enabled() {
		w, h = win.CanvasW*r.opts.Scale, win.CanvasH*r.opts.Scale
	}
	if dst == nil || dst.Bounds().Dx() != w || dst.Bounds().Dy() != h {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}

	placements := splitLayers(f.Graphics.Placements)

	// 1. Theme background — or the full window chrome (margin fill, bar,
	// terminal background incl. the visual padding) when configured.
	if r.opts.Window.enabled() {
		r.drawChrome(dst, rgba(f.Colors.BG), rgba(f.Colors.FG))
	} else {
		fillRect(dst, dst.Bounds(), rgba(f.Colors.BG))
	}
	// 2. Below-background placements, then explicit cell backgrounds.
	if err := r.drawPlacements(dst, src, placements[vtengine.LayerBelowBG]); err != nil {
		return nil, err
	}
	r.drawCellBackgrounds(dst, f)
	// 3. Below-text placements, text and decorations, above-text.
	if err := r.drawPlacements(dst, src, placements[vtengine.LayerBelowText]); err != nil {
		return nil, err
	}
	r.drawText(dst, f)
	if err := r.drawPlacements(dst, src, placements[vtengine.LayerAboveText]); err != nil {
		return nil, err
	}
	// 4. Cursor on top; the keys band's chips (they animate per frame,
	// unlike the band strip itself, which is chrome); rounded corners
	// re-reveal the margin fill LAST, masking the whole window block
	// like VHS does.
	r.drawCursor(dst, f)
	r.roundCorners(dst)
	// The key caps live OUTSIDE the window block (the band below), so
	// they composite after the corner mask.
	r.drawKeyChips(dst, f)
	return dst, nil
}

func splitLayers(ps []vtengine.Placement) map[vtengine.Layer][]vtengine.Placement {
	out := make(map[vtengine.Layer][]vtengine.Placement, 3)
	for _, p := range ps {
		if p.Virtual {
			continue // placeholder cells render via the grid, not here (fase 2)
		}
		out[p.Layer()] = append(out[p.Layer()], p)
	}
	return out
}

func (r *Rasterizer) drawCellBackgrounds(dst *image.RGBA, f *vtengine.Frame) {
	for y := 0; y < f.Geometry.Rows; y++ {
		for x := 0; x < f.Geometry.Cols; x++ {
			st := f.CellAt(x, y).Style
			bg, _ := effectiveColors(st, f)
			if st.HasBG || st.Inverse {
				fillRect(dst, r.cellRect(x, y, 1), bg)
			}
		}
	}
}

// effectiveColors applies inverse video: returns (bg, fg) to paint with.
func effectiveColors(st vtengine.Style, f *vtengine.Frame) (bg, fg color.RGBA) {
	fgc, bgc := st.FG, st.BG
	if !st.HasBG {
		bgc = f.Colors.BG
	}
	if st.Inverse {
		fgc, bgc = bgc, fgc
	}
	if st.Invisible {
		fgc = bgc
	}
	return rgba(bgc), rgba(fgc)
}

func (r *Rasterizer) cellRect(x, y, cells int) image.Rectangle {
	return image.Rect(r.orgX+x*r.cellW, r.orgY+y*r.cellH,
		r.orgX+(x+cells)*r.cellW, r.orgY+(y+1)*r.cellH)
}

func rgba(c vtengine.RGB) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: 0xff}
}

func fillRect(dst *image.RGBA, rect image.Rectangle, c color.RGBA) {
	rect = rect.Intersect(dst.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		i := dst.PixOffset(rect.Min.X, y)
		for x := rect.Min.X; x < rect.Max.X; x++ {
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = c.R, c.G, c.B, c.A
			i += 4
		}
	}
}
