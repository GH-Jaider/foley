package vtengine

import "math"

// Layer classifies a placement's z-index per kitty protocol conventions.
type Layer uint8

// Layers, in paint order.
const (
	LayerBelowBG Layer = iota
	LayerBelowText
	LayerAboveText
)

// Placement is one kitty-graphics placement visible in the viewport, with
// all geometry resolved by the engine (source rect clamped, viewport
// position computed, pixel size accounting for cols/rows scaling).
type Placement struct {
	ImageID     uint32
	PlacementID uint32

	// Virtual placements (unicode placeholders) carry no geometry of
	// their own; the rasterizer resolves them via placeholder cells.
	Virtual bool

	Z int32

	// Col and Row are viewport-relative and may be negative when the
	// placement is partially scrolled out; the rasterizer clips.
	Col, Row int32

	// OffX and OffY are pixel offsets within the anchor cell.
	OffX, OffY uint32

	// PixelW and PixelH are the final rendered size in pixels.
	PixelW, PixelH uint32

	// Source rectangle within the image, in pixels, already clamped.
	SrcX, SrcY, SrcW, SrcH uint32
}

// Layer returns the paint layer for the placement's z-index: below the
// background for z < INT32_MIN/2, between background and text for
// negative z, above text otherwise.
func (p Placement) Layer() Layer {
	switch {
	case p.Z < math.MinInt32/2:
		return LayerBelowBG
	case p.Z < 0:
		return LayerBelowText
	default:
		return LayerAboveText
	}
}

// Graphics is the kitty-graphics portion of a Frame.
type Graphics struct {
	// Generation changes on any transmit, placement or delete. Equal
	// generations guarantee identical placements and image data, so
	// renderers can skip snapshotting and cache validation entirely.
	Generation uint64

	// Placements visible this frame, unordered; sort by Layer then Z
	// when painting.
	Placements []Placement
}

// ImageData is the decoded pixel data of one stored image. Pix is always
// non-premultiplied RGBA, row-major, len == 4*W*H. The slice borrows
// engine-owned memory: valid only until the next Write/Resize/Close.
type ImageData struct {
	ID         uint32
	W, H       int
	Generation uint64
	Pix        []byte
}
