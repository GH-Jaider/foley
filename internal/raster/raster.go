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

// Options configures a Rasterizer.
type Options struct {
	Pack *fontpack.Pack
	// FontSizePx is the glyph size in pixels at Scale 1.
	FontSizePx int
	// Scale multiplies every metric (2 = native supersampling).
	Scale int
}

// Rasterizer turns engine frames into RGBA images. It caches parsed
// faces, glyph masks and decoded images; it is not safe for concurrent
// use (one rasterizer per recording, like the engine).
type Rasterizer struct {
	opts Options

	text, bold, italic, boldItalic *font.Face
	emoji                          *font.Face

	sizePx    int // FontSizePx * Scale
	cellW     int
	cellH     int
	baseline  int
	underline int // y offset from cell top
	thickness int

	shaper shaping.HarfbuzzShaper // reused: it caches font data internally

	glyphs map[glyphKey]*glyphMask
	emojis map[font.GID]*image.RGBA
	kitty  map[kittyKey]*image.RGBA
}

type glyphKey struct {
	style faceStyle
	gid   font.GID
}

type kittyKey struct {
	id  uint32
	gen uint64
}

type faceStyle uint8

const (
	faceRegular faceStyle = iota
	faceBold
	faceItalic
	faceBoldItalic
)

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
		opts:   opts,
		sizePx: opts.FontSizePx * opts.Scale,
		glyphs: make(map[glyphKey]*glyphMask),
		emojis: make(map[font.GID]*image.RGBA),
		kitty:  make(map[kittyKey]*image.RGBA),
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
	r.computeMetrics()
	return r, nil
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
	if dst == nil || dst.Bounds().Dx() != w || dst.Bounds().Dy() != h {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}

	placements := splitLayers(f.Graphics.Placements)

	// 1. Theme background.
	fillRect(dst, dst.Bounds(), rgba(f.Colors.BG))
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
	// 4. Cursor on top.
	r.drawCursor(dst, f)
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
	return image.Rect(x*r.cellW, y*r.cellH, (x+cells)*r.cellW, (y+1)*r.cellH)
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
