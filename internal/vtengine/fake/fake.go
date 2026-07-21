// Package fake is a puppet vtengine for driver and raster tests: state is
// set directly through Set* helpers, and Write applies only a trivial VT
// subset (printable ASCII, CR, LF) so timeline tests can drive it like a
// real engine. It passes enginetest.RunBasic; full VT conformance
// (enginetest.RunFull) is for real engines.
package fake

import (
	"fmt"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/key"
)

// Engine is the puppet. Construct with New; zero value is not usable.
type Engine struct {
	geo    vtengine.Geometry
	cells  []vtengine.Cell
	cursor vtengine.Cursor
	colors vtengine.Colors
	gfx    vtengine.Graphics
	title  string
	images map[uint32]vtengine.ImageData

	dirty  bool
	closed bool

	// Written records every byte fed via Write, for input-path tests.
	Written []byte

	// Scrolled records every ScrollViewport delta, for driver tests —
	// the puppet has no scrollback, so the call only marks dirty.
	Scrolled []int

	// EncodeKeyFunc overrides key encoding when set; the default encodes
	// printable runes as themselves and named keys as legacy sequences.
	EncodeKeyFunc func(vtengine.KeyEvent) ([]byte, error)
}

// New returns a puppet engine. Options.Colors seeds the snapshot colors;
// Options.Responses is accepted but never written to — the mini-VT answers
// no queries (real engines do; see enginetest.RunFull).
func New(opts vtengine.Options) *Engine {
	e := &Engine{images: make(map[uint32]vtengine.ImageData)}
	e.reset(opts.Geometry)
	if opts.Colors != nil {
		e.colors = *opts.Colors
	}
	return e
}

func (e *Engine) reset(g vtengine.Geometry) {
	e.geo = g
	e.cells = make([]vtengine.Cell, g.Cols*g.Rows)
	e.cursor = vtengine.Cursor{Visible: true}
	e.dirty = true
}

// Write applies printable ASCII, '\n' and '\r'; everything else is
// recorded but ignored. Lines past the last row overwrite the last row
// (the fake does not scroll — fixtures fit their grid).
func (e *Engine) Write(p []byte) (int, error) {
	if e.closed {
		return 0, vtengine.ErrClosed
	}
	e.Written = append(e.Written, p...)
	for _, b := range p {
		switch {
		case b == '\n':
			if e.cursor.Y < e.geo.Rows-1 {
				e.cursor.Y++
			}
		case b == '\r':
			e.cursor.X = 0
		case b >= 0x20 && b < 0x7f:
			if e.cursor.X >= e.geo.Cols {
				e.cursor.X = 0
				if e.cursor.Y < e.geo.Rows-1 {
					e.cursor.Y++
				}
			}
			e.SetCell(e.cursor.X, e.cursor.Y, string(rune(b)), vtengine.Style{})
			e.cursor.X++
		}
	}
	e.dirty = true
	return len(p), nil
}

// Resize resets the grid to the new geometry (contents are dropped; the
// fake keeps semantics minimal).
func (e *Engine) Resize(g vtengine.Geometry) error {
	if e.closed {
		return vtengine.ErrClosed
	}
	e.reset(g)
	return nil
}

// Snapshot copies the puppet state into dst.
func (e *Engine) Snapshot(dst *vtengine.Frame) error {
	if e.closed {
		return vtengine.ErrClosed
	}
	dst.Geometry = e.geo
	if cap(dst.Cells) < len(e.cells) {
		dst.Cells = make([]vtengine.Cell, len(e.cells))
	}
	dst.Cells = dst.Cells[:len(e.cells)]
	copy(dst.Cells, e.cells)
	dst.Cursor = e.cursor
	// Phantom column: after writing the last column the internal cursor
	// sits at Cols; observable state clamps to the last column, like a
	// real terminal's cursor position report.
	if dst.Cursor.X >= e.geo.Cols {
		dst.Cursor.X = e.geo.Cols - 1
	}
	dst.Colors = e.colors
	// Mimic real engines: an unset (zero) cursor color resolves to FG.
	if dst.Colors.Cursor == (vtengine.RGB{}) {
		dst.Colors.Cursor = dst.Colors.FG
	}
	dst.Dirty = e.dirty
	dst.Title = e.title
	dst.Graphics = vtengine.Graphics{
		Generation: e.gfx.Generation,
		Placements: append(dst.Graphics.Placements[:0], e.gfx.Placements...),
	}
	e.dirty = false
	return nil
}

// SetTitle sets the OSC-declared window title the next Snapshot reports
// and marks the frame dirty, like a real engine would.
func (e *Engine) SetTitle(title string) {
	e.title = title
	e.dirty = true
}

// ImagePixels returns pixels registered via SetImage.
func (e *Engine) ImagePixels(id uint32) (vtengine.ImageData, error) {
	if e.closed {
		return vtengine.ImageData{}, vtengine.ErrClosed
	}
	img, ok := e.images[id]
	if !ok {
		return vtengine.ImageData{}, vtengine.ErrNoImage
	}
	return img, nil
}

// ScrollViewport records the delta and marks the frame dirty (the
// contract: a moved viewport is a visible change). The puppet keeps no
// scrollback — real scroll behavior is enginetest.RunFull territory.
func (e *Engine) ScrollViewport(delta int) error {
	if e.closed {
		return vtengine.ErrClosed
	}
	e.Scrolled = append(e.Scrolled, delta)
	e.dirty = true
	return nil
}

// EncodeKey encodes taps with a trivial legacy scheme (overridable via
// EncodeKeyFunc).
func (e *Engine) EncodeKey(ev vtengine.KeyEvent) ([]byte, error) {
	if e.closed {
		return nil, vtengine.ErrClosed
	}
	if e.EncodeKeyFunc != nil {
		return e.EncodeKeyFunc(ev)
	}
	k := ev.Key
	if k.Rune != 0 {
		b := []byte(string(k.Rune))
		if k.Mods&key.ModCtrl != 0 && k.Rune >= 'a' && k.Rune <= 'z' {
			b = []byte{byte(k.Rune) & 0x1f} // legacy ctrl-letter
		}
		if k.Mods&key.ModAlt != 0 {
			b = append([]byte{0x1b}, b...) // legacy alt = ESC prefix
		}
		return b, nil
	}
	switch k.Name {
	case key.NameEnter:
		return []byte("\r"), nil
	case key.NameTab:
		return []byte("\t"), nil
	case key.NameSpace:
		return []byte(" "), nil
	case key.NameEscape:
		return []byte("\x1b"), nil
	case key.NameBackspace:
		return []byte("\x7f"), nil
	case key.NameUp:
		return []byte("\x1b[A"), nil
	case key.NameDown:
		return []byte("\x1b[B"), nil
	case key.NameRight:
		return []byte("\x1b[C"), nil
	case key.NameLeft:
		return []byte("\x1b[D"), nil
	case key.NameNone, key.NameDelete, key.NameInsert, key.NameHome,
		key.NameEnd, key.NamePageUp, key.NamePageDown:
		return nil, fmt.Errorf("%w: fake does not encode %v", vtengine.ErrCannotEncode, k.Name)
	default:
		return nil, fmt.Errorf("%w: fake does not encode %v", vtengine.ErrCannotEncode, k.Name)
	}
}

// Close is idempotent.
func (e *Engine) Close() error {
	e.closed = true
	return nil
}

// --- puppet controls (test-side state injection) ---

// SetCell writes a grapheme with a style at (x, y) and marks the frame
// dirty.
func (e *Engine) SetCell(x, y int, grapheme string, st vtengine.Style) {
	e.SetGrapheme(x, y, grapheme, 1, st)
}

// SetGrapheme writes a grapheme with an explicit cell width (2 = wide,
// leaving the following spacer cell empty), for raster tests.
func (e *Engine) SetGrapheme(x, y int, grapheme string, width int, st vtengine.Style) {
	// Mimic real engines: an unset (zero) underline color resolves to FG.
	if st.UnderlineColor == (vtengine.RGB{}) {
		st.UnderlineColor = st.FG
	}
	c := &e.cells[y*e.geo.Cols+x]
	c.Runes = []rune(grapheme)
	c.Width = uint8(width) //nolint:gosec // puppet control, test-sized values
	c.Style = st
	e.dirty = true
}

// SetCursor moves the puppet cursor.
func (e *Engine) SetCursor(c vtengine.Cursor) {
	e.cursor = c
	e.dirty = true
}

// SetColors sets terminal-level colors.
func (e *Engine) SetColors(c vtengine.Colors) {
	e.colors = c
	e.dirty = true
}

// AddPlacement registers a placement and bumps the graphics generation.
func (e *Engine) AddPlacement(p vtengine.Placement) {
	e.gfx.Placements = append(e.gfx.Placements, p)
	e.gfx.Generation++
	e.dirty = true
}

// SetImage registers image pixels for ImagePixels lookups.
func (e *Engine) SetImage(img vtengine.ImageData) {
	e.images[img.ID] = img
	e.gfx.Generation++
	e.dirty = true
}

var _ vtengine.Engine = (*Engine)(nil)
