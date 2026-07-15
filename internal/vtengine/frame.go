package vtengine

import "strings"

// RGB is a resolved 8-bit color. Engines resolve palette and default
// colors at snapshot time so consumers never see indexed colors.
type RGB struct{ R, G, B uint8 }

// UnderlineStyle enumerates underline shapes (kitty extensions included).
type UnderlineStyle uint8

// Underline styles.
const (
	UnderlineNone UnderlineStyle = iota
	UnderlineSingle
	UnderlineDouble
	UnderlineCurly
	UnderlineDotted
	UnderlineDashed
)

// Style is the complete resolved visual style of one cell.
type Style struct {
	FG             RGB
	BG             RGB
	UnderlineColor RGB
	Underline      UnderlineStyle

	// HasBG reports an explicitly set background: the rasterizer paints
	// only these over the theme background.
	HasBG bool

	Bold          bool
	Italic        bool
	Faint         bool
	Blink         bool
	Inverse       bool
	Invisible     bool
	Strikethrough bool
	Overline      bool
}

// Cell is one grid cell. A wide grapheme occupies Width columns; the
// following spacer cell has len(Runes) == 0 and Width == 0.
type Cell struct {
	// Runes is the grapheme cluster for this cell (empty for blank and
	// spacer cells). The slice may alias engine-managed storage: valid
	// until the next Snapshot into the same Frame.
	Runes []rune
	Width uint8
	Style Style
}

// CursorShape enumerates cursor visual styles.
type CursorShape uint8

// Cursor shapes.
const (
	CursorBlock CursorShape = iota
	CursorBar
	CursorUnderline
	CursorHollowBlock
)

// Cursor is the cursor state within the viewport.
type Cursor struct {
	X, Y    int
	Visible bool
	Shape   CursorShape
}

// Colors are the terminal-level colors active at snapshot time.
type Colors struct {
	FG, BG  RGB
	Palette [256]RGB
}

// Frame is a complete, self-contained snapshot of the terminal state for
// one render instant. It is designed for reuse: pass the same Frame to
// successive Snapshot calls to avoid allocations.
type Frame struct {
	Geometry Geometry

	// Cells is row-major, len == Cols*Rows.
	Cells []Cell

	Cursor Cursor
	Colors Colors

	// Dirty reports whether anything changed since the previous Snapshot
	// on this engine; a false value lets render loops skip work entirely.
	Dirty bool

	Graphics Graphics
}

// CellAt returns the cell at column x, row y. It panics on out-of-range
// coordinates, matching slice semantics.
func (f *Frame) CellAt(x, y int) *Cell {
	return &f.Cells[y*f.Geometry.Cols+x]
}

// RowText returns the visible text of row y with trailing blanks trimmed.
// Blank and spacer cells inside the line become spaces.
func (f *Frame) RowText(y int) string {
	var b strings.Builder
	for x := 0; x < f.Geometry.Cols; x++ {
		c := f.CellAt(x, y)
		switch {
		case len(c.Runes) > 0:
			for _, r := range c.Runes {
				b.WriteRune(r)
			}
		case c.Width == 0 && x > 0 && f.CellAt(x-1, y).Width == 2:
			// spacer after a wide grapheme: contributes nothing
		default:
			b.WriteByte(' ')
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Text returns the visible screen text, one line per row, with trailing
// blank rows trimmed. It is the surface Wait+Screen regexes match against.
func (f *Frame) Text() string {
	lines := make([]string, f.Geometry.Rows)
	last := -1
	for y := 0; y < f.Geometry.Rows; y++ {
		lines[y] = f.RowText(y)
		if lines[y] != "" {
			last = y
		}
	}
	return strings.Join(lines[:last+1], "\n")
}
