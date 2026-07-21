//go:build ghosttyvt

package ghostty

/*
#cgo CFLAGS: -I${SRCDIR}/include
#cgo darwin,arm64 LDFLAGS: ${SRCDIR}/lib/darwin-arm64/libghostty-vt.a
#cgo darwin,amd64 LDFLAGS: ${SRCDIR}/lib/darwin-amd64/libghostty-vt.a
#cgo linux,arm64 LDFLAGS: ${SRCDIR}/lib/linux-arm64/libghostty-vt.a
#cgo linux,amd64 LDFLAGS: ${SRCDIR}/lib/linux-amd64/libghostty-vt.a
#include <stdlib.h>
#include <string.h>
#include <ghostty/vt.h>

// Exported from Go (see below). Declared with the exact prototypes cgo
// generates (no const qualifiers); the helpers cast to the callback
// typedefs, which only add const.
extern void foleyWritePty(GhosttyTerminal term, void* userdata, uint8_t* data, size_t len);
extern bool foleyDecodePng(void* userdata, GhosttyAllocator* allocator, uint8_t* data, size_t data_len, GhosttySysImage* out);

static void foley_install_write_pty(GhosttyTerminal t, void* userdata) {
	GhosttyTerminalWritePtyFn fn = (GhosttyTerminalWritePtyFn)foleyWritePty;
	ghostty_terminal_set(t, GHOSTTY_TERMINAL_OPT_USERDATA, userdata);
	ghostty_terminal_set(t, GHOSTTY_TERMINAL_OPT_WRITE_PTY, (const void*)fn);
}

static void foley_install_png_decoder(void) {
	GhosttySysDecodePngFn fn = (GhosttySysDecodePngFn)foleyDecodePng;
	ghostty_sys_set(GHOSTTY_SYS_OPT_DECODE_PNG, (const void*)fn);
}
*/
import "C"

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"runtime/cgo"
	"sync"
	"unsafe"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/key"
)

// Engine implements vtengine.Engine on libghostty-vt (the pinned static
// library; see libbuild/).
type Engine struct {
	term    C.GhosttyTerminal
	rstate  C.GhosttyRenderState
	rowIter C.GhosttyRenderStateRowIterator
	cells   C.GhosttyRenderStateRowCells
	gfxIter C.GhosttyKittyGraphicsPlacementIterator
	encoder C.GhosttyKeyEncoder
	keyEv   C.GhosttyKeyEvent

	handle cgo.Handle // self-reference passed to C callbacks as userdata
	geo    vtengine.Geometry
	opts   vtengine.Options
	closed bool
	// qf answers the queries the lib will not (queries.go); its state
	// carries partial sequences across Write boundaries.
	qf queryFilter
	// lastTitle detects OSC 0/2 title changes between snapshots: a pure
	// title change carries no dirty cells, but the frame must not be
	// skipped.
	lastTitle string
	// lastGfxGen detects kitty-graphics mutations between snapshots: an
	// animation retransmitting frames over the protocol never touches
	// cell damage, so the lib's dirty flag stays FALSE while the screen
	// visibly moves (found live: a realtime take of tenten froze on one
	// frame). The generation counter is graphics' own dirty bit — same
	// doctrine as lastTitle.
	lastGfxGen uint64
}

//nolint:gochecknoglobals // process-wide C callback registration must happen exactly once
var installPNGDecoder sync.Once

// New creates a libghostty-vt engine.
func New(opts vtengine.Options) (*Engine, error) {
	installPNGDecoder.Do(func() { C.foley_install_png_decoder() })

	e := &Engine{geo: opts.Geometry, opts: opts}

	copts := C.GhosttyTerminalOptions{
		cols: C.uint16_t(opts.Geometry.Cols),
		rows: C.uint16_t(opts.Geometry.Rows),
		// Scrollback MUST be nonzero even though foley never renders
		// history: kitty placements are anchored by tracked pins, and
		// with zero scrollback a scrolled-off anchor line is DESTROYED —
		// ghostty re-clamps the orphaned pin to the top of the page, so
		// the image parks over the viewport forever instead of scrolling
		// away (found live: the foley welcome logo painting over its own
		// help text). With history the pin scrolls out like in any real
		// terminal: negative viewport rows, then not visible. Units are
		// lines; the cap only bounds memory on long demos.
		max_scrollback: 10_000,
	}
	if rc := C.ghostty_terminal_new(nil, &e.term, copts); rc != C.GHOSTTY_SUCCESS {
		return nil, fmt.Errorf("ghostty: terminal_new failed: rc=%d", int(rc))
	}

	ok := false
	defer func() {
		if !ok {
			e.freeAll()
		}
	}()

	if err := e.applyGeometry(opts.Geometry); err != nil {
		return nil, err
	}

	if opts.KittyStorageLimit > 0 {
		limit := C.uint64_t(opts.KittyStorageLimit)
		C.ghostty_terminal_set(e.term, C.GHOSTTY_TERMINAL_OPT_KITTY_IMAGE_STORAGE_LIMIT,
			unsafe.Pointer(&limit))
	}

	if opts.Colors != nil {
		fg := rgbC(opts.Colors.FG)
		bg := rgbC(opts.Colors.BG)
		C.ghostty_terminal_set(e.term, C.GHOSTTY_TERMINAL_OPT_COLOR_FOREGROUND, unsafe.Pointer(&fg))
		C.ghostty_terminal_set(e.term, C.GHOSTTY_TERMINAL_OPT_COLOR_BACKGROUND, unsafe.Pointer(&bg))
		// A zero Cursor follows FG (Colors contract); seeding the resolved
		// value keeps OSC 112 resets landing on it as the default.
		cur := rgbC(opts.Colors.Cursor)
		if opts.Colors.Cursor == (vtengine.RGB{}) {
			cur = fg
		}
		C.ghostty_terminal_set(e.term, C.GHOSTTY_TERMINAL_OPT_COLOR_CURSOR, unsafe.Pointer(&cur))
		var pal [256]C.GhosttyColorRgb
		for i, c := range opts.Colors.Palette {
			pal[i] = rgbC(c)
		}
		C.ghostty_terminal_set(e.term, C.GHOSTTY_TERMINAL_OPT_COLOR_PALETTE, unsafe.Pointer(&pal))
	}

	e.handle = cgo.NewHandle(e)
	C.foley_install_write_pty(e.term, unsafe.Pointer(&e.handle))

	if rc := C.ghostty_render_state_new(nil, &e.rstate); rc != C.GHOSTTY_SUCCESS {
		return nil, fmt.Errorf("ghostty: render_state_new failed: rc=%d", int(rc))
	}
	if rc := C.ghostty_render_state_row_iterator_new(nil, &e.rowIter); rc != C.GHOSTTY_SUCCESS {
		return nil, fmt.Errorf("ghostty: row_iterator_new failed: rc=%d", int(rc))
	}
	if rc := C.ghostty_render_state_row_cells_new(nil, &e.cells); rc != C.GHOSTTY_SUCCESS {
		return nil, fmt.Errorf("ghostty: row_cells_new failed: rc=%d", int(rc))
	}
	if rc := C.ghostty_kitty_graphics_placement_iterator_new(nil, &e.gfxIter); rc != C.GHOSTTY_SUCCESS {
		return nil, fmt.Errorf("ghostty: placement_iterator_new failed: rc=%d", int(rc))
	}
	if rc := C.ghostty_key_encoder_new(nil, &e.encoder); rc != C.GHOSTTY_SUCCESS {
		return nil, fmt.Errorf("ghostty: key_encoder_new failed: rc=%d", int(rc))
	}
	if rc := C.ghostty_key_event_new(nil, &e.keyEv); rc != C.GHOSTTY_SUCCESS {
		return nil, fmt.Errorf("ghostty: key_event_new failed: rc=%d", int(rc))
	}

	ok = true
	return e, nil
}

func rgbC(c vtengine.RGB) C.GhosttyColorRgb {
	return C.GhosttyColorRgb{r: C.uint8_t(c.R), g: C.uint8_t(c.G), b: C.uint8_t(c.B)}
}

func rgbGo(c C.GhosttyColorRgb) vtengine.RGB {
	return vtengine.RGB{R: uint8(c.r), G: uint8(c.g), B: uint8(c.b)}
}

func (e *Engine) applyGeometry(g vtengine.Geometry) error {
	rc := C.ghostty_terminal_resize(e.term,
		C.uint16_t(g.Cols), C.uint16_t(g.Rows),
		C.uint32_t(g.CellW), C.uint32_t(g.CellH))
	if rc != C.GHOSTTY_SUCCESS {
		return fmt.Errorf("ghostty: resize failed: rc=%d", int(rc))
	}
	e.geo = g
	return nil
}

// Write feeds raw pty output bytes to the VT parser. Terminal query
// responses arrive synchronously on Options.Responses via the WRITE_PTY
// callback; the queries the lib leaves silent (XTWINOPS geometry,
// XTGETTCAP — see queries.go) are answered here, interleaved in stream
// order: the lib is fed up to the end of each matched query before its
// answer is written.
func (e *Engine) Write(p []byte) (int, error) {
	if e.closed {
		return 0, vtengine.ErrClosed
	}
	total := len(p)
	for len(p) > 0 {
		n, ev := e.qf.scan(p)
		if n > 0 {
			C.ghostty_terminal_vt_write(e.term,
				(*C.uint8_t)(unsafe.Pointer(&p[0])), C.size_t(n))
		}
		if ev != nil {
			e.answerQuery(ev)
		}
		p = p[n:]
	}
	return total, nil
}

// Resize changes grid and cell geometry.
func (e *Engine) Resize(g vtengine.Geometry) error {
	if e.closed {
		return vtengine.ErrClosed
	}
	return e.applyGeometry(g)
}

// Snapshot fills dst from the current render state.
func (e *Engine) Snapshot(dst *vtengine.Frame) error {
	if e.closed {
		return vtengine.ErrClosed
	}
	if rc := C.ghostty_render_state_update(e.rstate, e.term); rc != C.GHOSTTY_SUCCESS {
		return fmt.Errorf("ghostty: render_state_update failed: rc=%d", int(rc))
	}

	dst.Geometry = e.geo
	n := e.geo.Cols * e.geo.Rows
	if cap(dst.Cells) < n {
		dst.Cells = make([]vtengine.Cell, n)
	}
	dst.Cells = dst.Cells[:n]
	for i := range dst.Cells {
		dst.Cells[i] = vtengine.Cell{}
	}

	// Dirty (and reset it for the next frame).
	var dirty C.GhosttyRenderStateDirty
	C.ghostty_render_state_get(e.rstate, C.GHOSTTY_RENDER_STATE_DATA_DIRTY, unsafe.Pointer(&dirty))
	dst.Dirty = dirty != C.GHOSTTY_RENDER_STATE_DIRTY_FALSE
	clean := C.GhosttyRenderStateDirty(C.GHOSTTY_RENDER_STATE_DIRTY_FALSE)
	C.ghostty_render_state_set(e.rstate, C.GHOSTTY_RENDER_STATE_OPTION_DIRTY, unsafe.Pointer(&clean))

	// Colors.
	var colors C.GhosttyRenderStateColors
	colors.size = C.size_t(unsafe.Sizeof(colors))
	if rc := C.ghostty_render_state_colors_get(e.rstate, &colors); rc == C.GHOSTTY_SUCCESS {
		dst.Colors.FG = rgbGo(colors.foreground)
		dst.Colors.BG = rgbGo(colors.background)
		for i := range dst.Colors.Palette {
			dst.Colors.Palette[i] = rgbGo(colors.palette[i])
		}
	}
	// Cursor color via the terminal getter: unlike the render-state struct
	// field (zero when unset — a black sentinel), it distinguishes "not
	// configured" (GHOSTTY_NO_VALUE) so the contract's FG fallback never
	// shadows an explicit black.
	var cursorRGB C.GhosttyColorRgb
	if rc := C.ghostty_terminal_get(e.term, C.GHOSTTY_TERMINAL_DATA_COLOR_CURSOR, unsafe.Pointer(&cursorRGB)); rc == C.GHOSTTY_SUCCESS {
		dst.Colors.Cursor = rgbGo(cursorRGB)
	} else {
		dst.Colors.Cursor = dst.Colors.FG
	}

	// Title (OSC 0/2): a borrowed string, len 0 until the app sets one
	// A change dirties the frame even with a quiet grid.
	var title C.GhosttyString
	dst.Title = ""
	if rc := C.ghostty_terminal_get(e.term, C.GHOSTTY_TERMINAL_DATA_TITLE, unsafe.Pointer(&title)); rc == C.GHOSTTY_SUCCESS && title.len > 0 {
		dst.Title = C.GoStringN((*C.char)(unsafe.Pointer(title.ptr)), C.int(title.len))
	}
	if dst.Title != e.lastTitle {
		e.lastTitle = dst.Title
		dst.Dirty = true
	}

	// Cursor.
	e.snapshotCursor(dst)

	// Grid.
	if rc := C.ghostty_render_state_get(e.rstate, C.GHOSTTY_RENDER_STATE_DATA_ROW_ITERATOR,
		unsafe.Pointer(&e.rowIter)); rc != C.GHOSTTY_SUCCESS {
		return fmt.Errorf("ghostty: row iterator get failed: rc=%d", int(rc))
	}
	y := 0
	for C.ghostty_render_state_row_iterator_next(e.rowIter) && y < e.geo.Rows {
		if rc := C.ghostty_render_state_row_get(e.rowIter, C.GHOSTTY_RENDER_STATE_ROW_DATA_CELLS,
			unsafe.Pointer(&e.cells)); rc != C.GHOSTTY_SUCCESS {
			return fmt.Errorf("ghostty: row cells get failed: rc=%d", int(rc))
		}
		x := 0
		for C.ghostty_render_state_row_cells_next(e.cells) && x < e.geo.Cols {
			e.snapshotCell(&dst.Cells[y*e.geo.Cols+x], &dst.Colors)
			x++
		}
		y++
	}

	// Kitty graphics.
	return e.snapshotGraphics(dst)
}

func (e *Engine) snapshotCursor(dst *vtengine.Frame) {
	var visible, inViewport C.bool
	C.ghostty_render_state_get(e.rstate, C.GHOSTTY_RENDER_STATE_DATA_CURSOR_VISIBLE,
		unsafe.Pointer(&visible))
	C.ghostty_render_state_get(e.rstate, C.GHOSTTY_RENDER_STATE_DATA_CURSOR_VIEWPORT_HAS_VALUE,
		unsafe.Pointer(&inViewport))
	dst.Cursor = vtengine.Cursor{Visible: bool(visible) && bool(inViewport)}
	if !dst.Cursor.Visible {
		return
	}
	var cx, cy C.uint16_t
	C.ghostty_render_state_get(e.rstate, C.GHOSTTY_RENDER_STATE_DATA_CURSOR_VIEWPORT_X, unsafe.Pointer(&cx))
	C.ghostty_render_state_get(e.rstate, C.GHOSTTY_RENDER_STATE_DATA_CURSOR_VIEWPORT_Y, unsafe.Pointer(&cy))
	dst.Cursor.X, dst.Cursor.Y = int(cx), int(cy)

	var style C.GhosttyRenderStateCursorVisualStyle
	C.ghostty_render_state_get(e.rstate, C.GHOSTTY_RENDER_STATE_DATA_CURSOR_VISUAL_STYLE,
		unsafe.Pointer(&style))
	switch style {
	case C.GHOSTTY_RENDER_STATE_CURSOR_VISUAL_STYLE_BAR:
		dst.Cursor.Shape = vtengine.CursorBar
	case C.GHOSTTY_RENDER_STATE_CURSOR_VISUAL_STYLE_UNDERLINE:
		dst.Cursor.Shape = vtengine.CursorUnderline
	case C.GHOSTTY_RENDER_STATE_CURSOR_VISUAL_STYLE_BLOCK_HOLLOW:
		dst.Cursor.Shape = vtengine.CursorHollowBlock
	default:
		dst.Cursor.Shape = vtengine.CursorBlock
	}
}

func (e *Engine) snapshotCell(c *vtengine.Cell, colors *vtengine.Colors) {
	// Width and spacer detection come from the raw cell.
	var raw C.GhosttyCell
	C.ghostty_render_state_row_cells_get(e.cells,
		C.GHOSTTY_RENDER_STATE_ROW_CELLS_DATA_RAW, unsafe.Pointer(&raw))
	var wide C.GhosttyCellWide
	C.ghostty_cell_get(raw, C.GHOSTTY_CELL_DATA_WIDE, unsafe.Pointer(&wide))
	switch wide {
	case C.GHOSTTY_CELL_WIDE_SPACER_TAIL, C.GHOSTTY_CELL_WIDE_SPACER_HEAD:
		return // spacer: no content, Width 0
	case C.GHOSTTY_CELL_WIDE_WIDE:
		c.Width = 2
	default:
		c.Width = 1
	}

	var glen C.uint32_t
	C.ghostty_render_state_row_cells_get(e.cells,
		C.GHOSTTY_RENDER_STATE_ROW_CELLS_DATA_GRAPHEMES_LEN, unsafe.Pointer(&glen))
	if glen == 0 {
		c.Width = 0
		return
	}
	var cps [16]C.uint32_t
	if glen > 16 {
		glen = 16
	}
	C.ghostty_render_state_row_cells_get(e.cells,
		C.GHOSTTY_RENDER_STATE_ROW_CELLS_DATA_GRAPHEMES_BUF, unsafe.Pointer(&cps[0]))
	c.Runes = c.Runes[:0]
	for i := 0; i < int(glen); i++ {
		c.Runes = append(c.Runes, rune(cps[i]))
	}

	var style C.GhosttyStyle
	style.size = C.size_t(unsafe.Sizeof(style))
	C.ghostty_render_state_row_cells_get(e.cells,
		C.GHOSTTY_RENDER_STATE_ROW_CELLS_DATA_STYLE, unsafe.Pointer(&style))
	c.Style = styleGo(style, colors)
}

func styleGo(s C.GhosttyStyle, colors *vtengine.Colors) vtengine.Style {
	out := vtengine.Style{
		Bold:          bool(s.bold),
		Italic:        bool(s.italic),
		Faint:         bool(s.faint),
		Blink:         bool(s.blink),
		Inverse:       bool(s.inverse),
		Invisible:     bool(s.invisible),
		Strikethrough: bool(s.strikethrough),
		Overline:      bool(s.overline),
	}
	switch s.underline {
	case C.GHOSTTY_SGR_UNDERLINE_SINGLE:
		out.Underline = vtengine.UnderlineSingle
	case C.GHOSTTY_SGR_UNDERLINE_DOUBLE:
		out.Underline = vtengine.UnderlineDouble
	case C.GHOSTTY_SGR_UNDERLINE_CURLY:
		out.Underline = vtengine.UnderlineCurly
	case C.GHOSTTY_SGR_UNDERLINE_DOTTED:
		out.Underline = vtengine.UnderlineDotted
	case C.GHOSTTY_SGR_UNDERLINE_DASHED:
		out.Underline = vtengine.UnderlineDashed
	default:
		out.Underline = vtengine.UnderlineNone
	}
	out.FG = resolveColor(s.fg_color, colors, colors.FG, nil)
	out.BG = resolveColor(s.bg_color, colors, colors.BG, &out.HasBG)
	out.UnderlineColor = resolveColor(s.underline_color, colors, out.FG, nil)
	return out
}

func resolveColor(c C.GhosttyStyleColor, colors *vtengine.Colors, fallback vtengine.RGB, set *bool) vtengine.RGB {
	switch c.tag {
	case C.GHOSTTY_STYLE_COLOR_RGB:
		if set != nil {
			*set = true
		}
		return rgbGo(*(*C.GhosttyColorRgb)(unsafe.Pointer(&c.value)))
	case C.GHOSTTY_STYLE_COLOR_PALETTE:
		if set != nil {
			*set = true
		}
		idx := *(*C.uint8_t)(unsafe.Pointer(&c.value))
		return colors.Palette[uint8(idx)]
	default:
		return fallback
	}
}

// effectiveBG reads the terminal's CURRENT background — including any
// OSC 11 override the app made mid-stream — for the color-scheme
// report (queries.go). The lib is the source of truth, never a stale
// copy of the seed colors.
func (e *Engine) effectiveBG() (vtengine.RGB, bool) {
	var c C.GhosttyColorRgb
	if rc := C.ghostty_terminal_get(e.term, C.GHOSTTY_TERMINAL_DATA_COLOR_BACKGROUND,
		unsafe.Pointer(&c)); rc != C.GHOSTTY_SUCCESS {
		return vtengine.RGB{}, false
	}
	return rgbGo(c), true
}

func (e *Engine) graphics() (C.GhosttyKittyGraphics, bool) {
	var gfx C.GhosttyKittyGraphics
	rc := C.ghostty_terminal_get(e.term, C.GHOSTTY_TERMINAL_DATA_KITTY_GRAPHICS,
		unsafe.Pointer(&gfx))
	return gfx, rc == C.GHOSTTY_SUCCESS && gfx != nil
}

func (e *Engine) snapshotGraphics(dst *vtengine.Frame) error {
	dst.Graphics.Placements = dst.Graphics.Placements[:0]
	defer func() {
		// Graphics mutations (transmit, place, delete, clear) bump the
		// generation without touching cell damage — without this, a
		// graphics-only change is an invisible frame (see lastGfxGen).
		if dst.Graphics.Generation != e.lastGfxGen {
			e.lastGfxGen = dst.Graphics.Generation
			dst.Dirty = true
		}
	}()
	gfx, ok := e.graphics()
	if !ok {
		dst.Graphics.Generation = 0
		return nil
	}
	var gen C.uint64_t
	C.ghostty_kitty_graphics_get(gfx, C.GHOSTTY_KITTY_GRAPHICS_DATA_GENERATION,
		unsafe.Pointer(&gen))
	dst.Graphics.Generation = uint64(gen)
	if gen == 0 {
		return nil
	}

	if rc := C.ghostty_kitty_graphics_get(gfx,
		C.GHOSTTY_KITTY_GRAPHICS_DATA_PLACEMENT_ITERATOR,
		unsafe.Pointer(&e.gfxIter)); rc != C.GHOSTTY_SUCCESS {
		return fmt.Errorf("ghostty: placement iterator get failed: rc=%d", int(rc))
	}
	for C.ghostty_kitty_graphics_placement_next(e.gfxIter) {
		var p vtengine.Placement
		var imageID, placementID C.uint32_t
		var virtual C.bool
		var z C.int32_t
		C.ghostty_kitty_graphics_placement_get(e.gfxIter,
			C.GHOSTTY_KITTY_GRAPHICS_PLACEMENT_DATA_IMAGE_ID, unsafe.Pointer(&imageID))
		C.ghostty_kitty_graphics_placement_get(e.gfxIter,
			C.GHOSTTY_KITTY_GRAPHICS_PLACEMENT_DATA_PLACEMENT_ID, unsafe.Pointer(&placementID))
		C.ghostty_kitty_graphics_placement_get(e.gfxIter,
			C.GHOSTTY_KITTY_GRAPHICS_PLACEMENT_DATA_IS_VIRTUAL, unsafe.Pointer(&virtual))
		C.ghostty_kitty_graphics_placement_get(e.gfxIter,
			C.GHOSTTY_KITTY_GRAPHICS_PLACEMENT_DATA_Z, unsafe.Pointer(&z))
		p.ImageID, p.PlacementID = uint32(imageID), uint32(placementID)
		p.Virtual, p.Z = bool(virtual), int32(z)
		if p.Virtual {
			// Placeholder placements render via the grid (fase 2):
			// they carry no paint geometry here.
			dst.Graphics.Placements = append(dst.Graphics.Placements, p)
			continue
		}

		// Sub-cell pixel offsets (kitty X=/Y=): apps center sprites with
		// these — dropping them shifts every placement to its cell corner
		// (found live by tenten, which centers via X/Y).
		var offX, offY C.uint32_t
		C.ghostty_kitty_graphics_placement_get(e.gfxIter,
			C.GHOSTTY_KITTY_GRAPHICS_PLACEMENT_DATA_X_OFFSET, unsafe.Pointer(&offX))
		C.ghostty_kitty_graphics_placement_get(e.gfxIter,
			C.GHOSTTY_KITTY_GRAPHICS_PLACEMENT_DATA_Y_OFFSET, unsafe.Pointer(&offY))
		p.OffX, p.OffY = uint32(offX), uint32(offY)

		img := C.ghostty_kitty_graphics_image(gfx, imageID)
		if img == nil {
			continue // image deleted from storage: nothing to show
		}
		var info C.GhosttyKittyGraphicsPlacementRenderInfo
		info.size = C.size_t(unsafe.Sizeof(info))
		if rc := C.ghostty_kitty_graphics_placement_render_info(e.gfxIter, img, e.term, &info); rc != C.GHOSTTY_SUCCESS {
			continue // no resolvable geometry: nothing to paint this frame
		}
		p.Col, p.Row = int32(info.viewport_col), int32(info.viewport_row)
		p.PixelW, p.PixelH = uint32(info.pixel_width), uint32(info.pixel_height)
		p.SrcX, p.SrcY = uint32(info.source_x), uint32(info.source_y)
		p.SrcW, p.SrcH = uint32(info.source_width), uint32(info.source_height)
		if !bool(info.viewport_visible) {
			continue // fully off-viewport: nothing to paint this frame
		}
		dst.Graphics.Placements = append(dst.Graphics.Placements, p)
	}
	return nil
}

// ImagePixels returns decoded RGBA pixels for a stored image. RGBA data is
// borrowed zero-copy from the engine (valid until the next mutation);
// other formats are converted into a fresh Go slice.
func (e *Engine) ImagePixels(id uint32) (vtengine.ImageData, error) {
	if e.closed {
		return vtengine.ImageData{}, vtengine.ErrClosed
	}
	gfx, ok := e.graphics()
	if !ok {
		return vtengine.ImageData{}, vtengine.ErrNoImage
	}
	img := C.ghostty_kitty_graphics_image(gfx, C.uint32_t(id))
	if img == nil {
		return vtengine.ImageData{}, vtengine.ErrNoImage
	}
	var w, h C.uint32_t
	var format C.GhosttyKittyImageFormat
	var dataPtr *C.uint8_t
	var dataLen C.size_t
	var gen C.uint64_t
	C.ghostty_kitty_graphics_image_get(img, C.GHOSTTY_KITTY_IMAGE_DATA_WIDTH, unsafe.Pointer(&w))
	C.ghostty_kitty_graphics_image_get(img, C.GHOSTTY_KITTY_IMAGE_DATA_HEIGHT, unsafe.Pointer(&h))
	C.ghostty_kitty_graphics_image_get(img, C.GHOSTTY_KITTY_IMAGE_DATA_FORMAT, unsafe.Pointer(&format))
	C.ghostty_kitty_graphics_image_get(img, C.GHOSTTY_KITTY_IMAGE_DATA_DATA_PTR, unsafe.Pointer(&dataPtr))
	C.ghostty_kitty_graphics_image_get(img, C.GHOSTTY_KITTY_IMAGE_DATA_DATA_LEN, unsafe.Pointer(&dataLen))
	C.ghostty_kitty_graphics_image_get(img, C.GHOSTTY_KITTY_IMAGE_DATA_GENERATION, unsafe.Pointer(&gen))

	out := vtengine.ImageData{ID: id, W: int(w), H: int(h), Generation: uint64(gen)}
	if dataPtr == nil || dataLen == 0 {
		return out, nil
	}
	raw := unsafe.Slice((*byte)(dataPtr), int(dataLen))
	switch format {
	case C.GHOSTTY_KITTY_IMAGE_FORMAT_RGBA:
		out.Pix = raw
	case C.GHOSTTY_KITTY_IMAGE_FORMAT_RGB:
		out.Pix = expandToRGBA(raw, 3, int(w)*int(h))
	case C.GHOSTTY_KITTY_IMAGE_FORMAT_GRAY_ALPHA:
		out.Pix = grayAlphaToRGBA(raw, int(w)*int(h))
	case C.GHOSTTY_KITTY_IMAGE_FORMAT_GRAY:
		out.Pix = grayToRGBA(raw, int(w)*int(h))
	default:
		return vtengine.ImageData{}, fmt.Errorf("ghostty: unexpected image format %d", int(format))
	}
	return out, nil
}

func expandToRGBA(raw []byte, stride, pixels int) []byte {
	out := make([]byte, pixels*4)
	for i := 0; i < pixels; i++ {
		copy(out[i*4:], raw[i*stride:i*stride+stride])
		out[i*4+3] = 0xff
	}
	return out
}

func grayAlphaToRGBA(raw []byte, pixels int) []byte {
	out := make([]byte, pixels*4)
	for i := 0; i < pixels; i++ {
		g, a := raw[i*2], raw[i*2+1]
		out[i*4], out[i*4+1], out[i*4+2], out[i*4+3] = g, g, g, a
	}
	return out
}

func grayToRGBA(raw []byte, pixels int) []byte {
	out := make([]byte, pixels*4)
	for i := 0; i < pixels; i++ {
		g := raw[i]
		out[i*4], out[i*4+1], out[i*4+2], out[i*4+3] = g, g, g, 0xff
	}
	return out
}

// EncodeKey encodes an input event per the terminal's active keyboard mode.
// EncodeKey encodes ev like a real terminal in the app's current mode,
// with two xterm-parity adjustments (empirically pinned by conformance):
// Shift folds out of otherwise-unencodable Ctrl/Alt+letter chords (xterm
// sends 0x03 for Ctrl+Shift+C), and — unless Options.ModifyOtherKeys —
// CSI-27 fallbacks degrade to the unmodified key (xterm/xterm.js send a
// plain \r for Ctrl+Enter; a migrated tape means THAT).
func (e *Engine) EncodeKey(ev vtengine.KeyEvent) ([]byte, error) {
	out, err := e.encodeKeyRaw(ev)
	if err != nil && ev.Key.Rune != 0 &&
		ev.Key.Mods&key.ModShift != 0 && ev.Key.Mods&(key.ModCtrl|key.ModAlt) != 0 {
		out, err = e.encodeKeyRaw(vtengine.KeyEvent{
			Key: key.Key{Rune: ev.Key.Rune, Name: ev.Key.Name, Mods: ev.Key.Mods &^ key.ModShift}, Type: ev.Type,
		})
	}
	if err != nil {
		return nil, err
	}
	if !e.opts.ModifyOtherKeys && bytes.HasPrefix(out, []byte("\x1b[27;")) {
		return e.encodeKeyRaw(vtengine.KeyEvent{
			Key: key.Key{Rune: ev.Key.Rune, Name: ev.Key.Name}, Type: ev.Type,
		})
	}
	return out, nil
}

func (e *Engine) encodeKeyRaw(ev vtengine.KeyEvent) ([]byte, error) {
	if e.closed {
		return nil, vtengine.ErrClosed
	}
	C.ghostty_key_encoder_setopt_from_terminal(e.encoder, e.term)
	// Alt must send ESC-prefix in legacy mode — the de-facto Linux
	// terminal default every tape assumes (VHS ran xterm.js where alt is
	// meta). Two knobs, both reset by setopt_from_terminal (the header
	// says so for option-as-alt): treat option AS alt, and prefix ESC.
	optionAsAlt := C.GhosttyOptionAsAlt(C.GHOSTTY_OPTION_AS_ALT_TRUE)
	C.ghostty_key_encoder_setopt(e.encoder,
		C.GHOSTTY_KEY_ENCODER_OPT_MACOS_OPTION_AS_ALT, unsafe.Pointer(&optionAsAlt))
	altEsc := C.bool(true)
	C.ghostty_key_encoder_setopt(e.encoder,
		C.GHOSTTY_KEY_ENCODER_OPT_ALT_ESC_PREFIX, unsafe.Pointer(&altEsc))
	// Third knob the refresh leaves in a non-xterm state (empirically:
	// Ctrl+Enter came out CSI-27 regardless of the app's XTMODKEYS
	// writes): pin it to the contract's ModifyOtherKeys choice.
	mok := C.bool(e.opts.ModifyOtherKeys)
	C.ghostty_key_encoder_setopt(e.encoder,
		C.GHOSTTY_KEY_ENCODER_OPT_MODIFY_OTHER_KEYS_STATE_2, unsafe.Pointer(&mok))

	var action C.GhosttyKeyAction
	switch ev.Type {
	case vtengine.KeyRelease:
		action = C.GHOSTTY_KEY_ACTION_RELEASE
	case vtengine.KeyTap, vtengine.KeyPress:
		action = C.GHOSTTY_KEY_ACTION_PRESS
	default:
		action = C.GHOSTTY_KEY_ACTION_PRESS
	}
	C.ghostty_key_event_set_action(e.keyEv, action)
	C.ghostty_key_event_set_mods(e.keyEv, modsC(ev.Key.Mods))
	C.ghostty_key_event_set_key(e.keyEv, keyC(ev.Key))

	// The encoder needs the codepoint to build text-carrying encodings:
	// plain/shifted runes, alt-prefixed ones (ESC+utf8 in legacy mode) —
	// and the Space key, which IS text (found by a real tape: bare Space
	// encoded empty). Ctrl stays keypress-only — its C0 byte derives
	// from the key (Ctrl+Space is NUL).
	textRune := ev.Key.Rune
	if textRune == 0 && ev.Key.Name == key.NameSpace {
		textRune = ' '
	}
	var utf8 []byte
	if textRune != 0 && ev.Key.Mods&^(key.ModShift|key.ModAlt) == 0 {
		utf8 = []byte(string(textRune))
		C.ghostty_key_event_set_utf8(e.keyEv,
			(*C.char)(unsafe.Pointer(&utf8[0])), C.size_t(len(utf8)))
	} else {
		C.ghostty_key_event_set_utf8(e.keyEv, nil, 0)
	}

	var buf [128]byte
	var written C.size_t
	rc := C.ghostty_key_encoder_encode(e.encoder, e.keyEv,
		(*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)), &written)
	if rc != C.GHOSTTY_SUCCESS {
		return nil, fmt.Errorf("%w: encoder rc=%d for %+v", vtengine.ErrCannotEncode, int(rc), ev.Key)
	}
	if written == 0 {
		return nil, fmt.Errorf("%w: empty encoding for %+v", vtengine.ErrCannotEncode, ev.Key)
	}
	return append([]byte(nil), buf[:written]...), nil
}

func modsC(m key.Mod) C.GhosttyMods {
	var out C.GhosttyMods
	if m&key.ModShift != 0 {
		out |= C.GHOSTTY_MODS_SHIFT
	}
	if m&key.ModCtrl != 0 {
		out |= C.GHOSTTY_MODS_CTRL
	}
	if m&key.ModAlt != 0 {
		out |= C.GHOSTTY_MODS_ALT
	}
	if m&key.ModSuper != 0 {
		out |= C.GHOSTTY_MODS_SUPER
	}
	return out
}

func keyC(k key.Key) C.GhosttyKey {
	switch k.Name {
	case key.NameEnter:
		return C.GHOSTTY_KEY_ENTER
	case key.NameEscape:
		return C.GHOSTTY_KEY_ESCAPE
	case key.NameBackspace:
		return C.GHOSTTY_KEY_BACKSPACE
	case key.NameTab:
		return C.GHOSTTY_KEY_TAB
	case key.NameSpace:
		return C.GHOSTTY_KEY_SPACE
	case key.NameDelete:
		return C.GHOSTTY_KEY_DELETE
	case key.NameInsert:
		return C.GHOSTTY_KEY_INSERT
	case key.NameUp:
		return C.GHOSTTY_KEY_ARROW_UP
	case key.NameDown:
		return C.GHOSTTY_KEY_ARROW_DOWN
	case key.NameLeft:
		return C.GHOSTTY_KEY_ARROW_LEFT
	case key.NameRight:
		return C.GHOSTTY_KEY_ARROW_RIGHT
	case key.NameHome:
		return C.GHOSTTY_KEY_HOME
	case key.NameEnd:
		return C.GHOSTTY_KEY_END
	case key.NamePageUp:
		return C.GHOSTTY_KEY_PAGE_UP
	case key.NamePageDown:
		return C.GHOSTTY_KEY_PAGE_DOWN
	case key.NameNone:
		return runeKeyC(k.Rune)
	default:
		return C.GHOSTTY_KEY_UNIDENTIFIED
	}
}

func runeKeyC(r rune) C.GhosttyKey {
	switch {
	case r >= 'a' && r <= 'z':
		return C.GhosttyKey(C.GHOSTTY_KEY_A + C.GhosttyKey(r-'a'))
	case r >= 'A' && r <= 'Z':
		return C.GhosttyKey(C.GHOSTTY_KEY_A + C.GhosttyKey(r-'A'))
	case r >= '0' && r <= '9':
		return C.GhosttyKey(C.GHOSTTY_KEY_DIGIT_0 + C.GhosttyKey(r-'0'))
	case r == ' ':
		return C.GHOSTTY_KEY_SPACE
	default:
		return C.GHOSTTY_KEY_UNIDENTIFIED
	}
}

// Close frees all engine resources. Idempotent.
func (e *Engine) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	e.freeAll()
	return nil
}

func (e *Engine) freeAll() {
	if e.keyEv != nil {
		C.ghostty_key_event_free(e.keyEv)
		e.keyEv = nil
	}
	if e.encoder != nil {
		C.ghostty_key_encoder_free(e.encoder)
		e.encoder = nil
	}
	if e.gfxIter != nil {
		C.ghostty_kitty_graphics_placement_iterator_free(e.gfxIter)
		e.gfxIter = nil
	}
	if e.cells != nil {
		C.ghostty_render_state_row_cells_free(e.cells)
		e.cells = nil
	}
	if e.rowIter != nil {
		C.ghostty_render_state_row_iterator_free(e.rowIter)
		e.rowIter = nil
	}
	if e.rstate != nil {
		C.ghostty_render_state_free(e.rstate)
		e.rstate = nil
	}
	if e.term != nil {
		C.ghostty_terminal_free(e.term)
		e.term = nil
	}
	if e.handle != 0 {
		e.handle.Delete()
		e.handle = 0
	}
}

var _ vtengine.Engine = (*Engine)(nil)

//export foleyWritePty
func foleyWritePty(_ C.GhosttyTerminal, userdata unsafe.Pointer, data *C.uint8_t, length C.size_t) {
	if userdata == nil || data == nil || length == 0 {
		return
	}
	h := *(*cgo.Handle)(userdata)
	e, ok := h.Value().(*Engine)
	if !ok || e.opts.Responses == nil {
		return
	}
	_, _ = e.opts.Responses.Write(C.GoBytes(unsafe.Pointer(data), C.int(length)))
}

//export foleyDecodePng
func foleyDecodePng(_ unsafe.Pointer, allocator *C.GhosttyAllocator, data *C.uint8_t, dataLen C.size_t, out *C.GhosttySysImage) C.bool {
	if data == nil || dataLen == 0 || out == nil {
		return false
	}
	src := C.GoBytes(unsafe.Pointer(data), C.int(dataLen))
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return false
	}
	b := img.Bounds()
	rgba := image.NewNRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)

	n := C.size_t(len(rgba.Pix))
	dst := C.ghostty_alloc(allocator, n)
	if dst == nil {
		return false
	}
	C.memcpy(unsafe.Pointer(dst), unsafe.Pointer(&rgba.Pix[0]), n)
	out.width = C.uint32_t(b.Dx())
	out.height = C.uint32_t(b.Dy())
	out.data = dst
	out.data_len = n
	return true
}
