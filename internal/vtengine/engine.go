package vtengine

import (
	"errors"
	"io"

	"github.com/GH-Jaider/foley/key"
)

// Sentinel errors.
var (
	// ErrClosed is returned by any operation on a closed engine.
	ErrClosed = errors.New("vtengine: engine is closed")
	// ErrUnknownEngine is returned by New for an unregistered engine name.
	ErrUnknownEngine = errors.New("vtengine: unknown engine")
	// ErrNoImage is returned by ImagePixels for an unknown image id.
	ErrNoImage = errors.New("vtengine: no such image")
	// ErrCannotEncode is returned (wrapped) by EncodeKey for events the
	// engine cannot encode. Engines never signal this by silently
	// returning empty bytes.
	ErrCannotEncode = errors.New("vtengine: cannot encode key event")
)

// Geometry is the terminal grid size plus the cell size in pixels. The
// pixel dimensions drive kitty-graphics placement geometry and must match
// what the rasterizer will draw with.
type Geometry struct {
	Cols, Rows   int
	CellW, CellH int
}

// Options configures a new engine instance.
type Options struct {
	Geometry Geometry

	// KittyStorageLimit is the kitty-graphics image storage budget in
	// bytes; zero disables the graphics subsystem.
	KittyStorageLimit uint64

	// Colors seeds the terminal's default foreground/background and
	// 256-color palette — this is how foley themes reach SGR palette
	// resolution. nil keeps the engine's built-in defaults.
	Colors *Colors

	// Responses receives the terminal's replies to the application:
	// device attributes, cursor position reports, kitty graphics ACKs
	// and keyboard-protocol query answers. The driver MUST pump these
	// bytes back into the application's pty — capability-probing TUIs
	// (yazi's kitty-graphics detection, for one) block waiting for
	// them. nil discards responses.
	Responses io.Writer

	// ModifyOtherKeys selects how modified keys WITHOUT a legacy form
	// (Ctrl+Enter, Ctrl+Shift+letter, ...) are encoded when the
	// application has not pushed a keyboard protocol. False — the
	// default — degrades exactly like xterm/xterm.js (Ctrl+Enter is a
	// plain Enter; Shift folds out of Ctrl+letter): what a VHS tape
	// meant. True keeps the modern xterm CSI-27 forms.
	ModifyOtherKeys bool
}

// KeyEventType distinguishes taps from explicit press/release events (the
// kitty keyboard protocol can report both edges).
type KeyEventType uint8

// Key event types.
const (
	// KeyTap is a logical press-and-release, the common case for tapes.
	KeyTap KeyEventType = iota
	// KeyPress is the press edge only.
	KeyPress
	// KeyRelease is the release edge only.
	KeyRelease
)

// KeyEvent is one input event to encode for the application.
type KeyEvent struct {
	Key  key.Key
	Type KeyEventType
}

// Engine is a headless terminal: it consumes the application's raw pty
// output (io.Writer) and owns the resulting state — grid, styles, cursor,
// and kitty-graphics storage. Implementations live behind the factory in
// this package; nothing outside internal/vtengine may import them
// (depguard).
//
// Engines are not safe for concurrent use; the driver serializes access.
type Engine interface {
	// Writer consumes raw VT bytes from the application's pty.
	io.Writer

	// Resize changes the grid and cell geometry.
	Resize(g Geometry) error

	// Snapshot fills dst with the current frame state, reusing dst's
	// backing storage where possible so a steady-state render loop does
	// not allocate. dst must not be nil; its previous contents are
	// discarded.
	Snapshot(dst *Frame) error

	// ImagePixels returns the decoded pixels for a kitty-graphics image
	// referenced by a Placement. The returned data is valid only until
	// the next Write, Resize or Close; callers cache by (ID, Generation).
	ImagePixels(id uint32) (ImageData, error)

	// EncodeKey encodes an input event exactly as a real terminal would
	// for the application's currently active keyboard mode (legacy or
	// kitty keyboard protocol). The returned bytes are written to the
	// application's pty by the caller. Unencodable events return an
	// error wrapping ErrCannotEncode — never silent empty bytes.
	EncodeKey(ev KeyEvent) ([]byte, error)

	// Close releases the engine. Further calls return ErrClosed; Close
	// itself is idempotent.
	Close() error
}

// Named engines are constructed by internal/vtengine/factory — a
// subpackage on purpose: every engine imports these contract types, so a
// factory in THIS package would be an import cycle.
