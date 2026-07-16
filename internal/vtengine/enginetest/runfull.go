package enginetest

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"regexp"
	"testing"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/key"
)

// RunFull runs RunBasic plus full VT conformance against a real engine:
// SGR styles, true color and palette resolution, wide graphemes, kitty
// graphics round-trips (raw and PNG, with generation stamps and straight
// alpha), keyboard-protocol-aware key encoding, and terminal query
// responses. Every assertion here was written against observed protocol
// behavior of the pinned engine, never guessed.
func RunFull(t *testing.T, factory Factory) {
	t.Helper()
	RunBasic(t, factory)

	t.Run("sgr_styles", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		mustWrite(t, e, "\x1b[1mB\x1b[0m\x1b[3mI\x1b[0m\x1b[9mS\x1b[0m\x1b[4:3mU\x1b[0m\x1b[7mR\x1b[0m\x1b[2mF\x1b[0m")
		f := snapshot(t, e)
		checks := []struct {
			x    int
			name string
			got  bool
		}{
			{0, "bold", f.CellAt(0, 0).Style.Bold},
			{1, "italic", f.CellAt(1, 0).Style.Italic},
			{2, "strikethrough", f.CellAt(2, 0).Style.Strikethrough},
			{3, "underline curly", f.CellAt(3, 0).Style.Underline == vtengine.UnderlineCurly},
			{4, "inverse", f.CellAt(4, 0).Style.Inverse},
			{5, "faint", f.CellAt(5, 0).Style.Faint},
		}
		for _, c := range checks {
			if !c.got {
				t.Errorf("cell %d: expected %s (style=%+v)", c.x, c.name, f.CellAt(c.x, 0).Style)
			}
		}
	})

	// The vtengine contract (Style.UnderlineColor) promises RESOLVED
	// values: equal to the effective FG when the app never asked for a
	// specific underline color (SGR 58), the requested color when it did,
	// and back to FG after SGR 59. Black underlines must stay paintable —
	// zero is never a sentinel.
	t.Run("underline_color_resolved", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		mustWrite(t, e, "\x1b[4m\x1b[38;2;10;20;30mA"+ // underline, no SGR 58
			"\x1b[58;2;200;100;50mB"+ // explicit underline color
			"\x1b[59mC\x1b[0m") // SGR 59: back to following FG
		f := snapshot(t, e)
		fg := vtengine.RGB{R: 10, G: 20, B: 30}
		if st := f.CellAt(0, 0).Style; st.UnderlineColor != fg || st.FG != fg {
			t.Errorf("unset underline color = %+v (FG %+v), want resolved == FG", st.UnderlineColor, st.FG)
		}
		if got := f.CellAt(1, 0).Style.UnderlineColor; got != (vtengine.RGB{R: 200, G: 100, B: 50}) {
			t.Errorf("explicit underline color = %+v, want the SGR 58 value", got)
		}
		if got := f.CellAt(2, 0).Style.UnderlineColor; got != fg {
			t.Errorf("underline color after SGR 59 = %+v, want FG %+v", got, fg)
		}
	})

	t.Run("true_color_and_palette", func(t *testing.T) {
		opts := defaultOpts()
		colors := vtengine.Colors{FG: vtengine.RGB{R: 200, G: 200, B: 200}}
		colors.Palette[42] = vtengine.RGB{R: 7, G: 42, B: 77}
		opts.Colors = &colors
		e := factory(t, opts)
		defer func() { _ = e.Close() }()
		mustWrite(t, e, "\x1b[38;2;10;20;30mX\x1b[0m\x1b[48;5;42mY\x1b[0m")
		f := snapshot(t, e)
		if got := f.CellAt(0, 0).Style.FG; got != (vtengine.RGB{R: 10, G: 20, B: 30}) {
			t.Errorf("true-color FG = %+v", got)
		}
		y := f.CellAt(1, 0).Style
		if !y.HasBG || y.BG != colors.Palette[42] {
			t.Errorf("palette BG = %+v (HasBG=%v), want %+v", y.BG, y.HasBG, colors.Palette[42])
		}
	})

	// Colors.Cursor is ALWAYS resolved: FG when nothing set a cursor
	// color, the OSC 12 override when the app set one, and back to the
	// default after OSC 112.
	t.Run("cursor_color_resolved", func(t *testing.T) {
		opts := defaultOpts()
		colors := vtengine.Colors{FG: vtengine.RGB{R: 200, G: 200, B: 200}}
		opts.Colors = &colors
		e := factory(t, opts)
		defer func() { _ = e.Close() }()
		f := snapshot(t, e)
		if f.Colors.Cursor != colors.FG {
			t.Fatalf("default cursor color = %+v, want FG %+v", f.Colors.Cursor, colors.FG)
		}
		mustWrite(t, e, "\x1b]12;rgb:c0/ff/ee\x07")
		f = snapshot(t, e)
		if f.Colors.Cursor != (vtengine.RGB{R: 0xc0, G: 0xff, B: 0xee}) {
			t.Fatalf("OSC 12 cursor color = %+v, want c0ffee", f.Colors.Cursor)
		}
		mustWrite(t, e, "\x1b]112\x07")
		f = snapshot(t, e)
		if f.Colors.Cursor != colors.FG {
			t.Fatalf("cursor color after OSC 112 = %+v, want FG %+v", f.Colors.Cursor, colors.FG)
		}
	})

	t.Run("wide_graphemes", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		mustWrite(t, e, "日a")
		f := snapshot(t, e)
		if w := f.CellAt(0, 0); string(w.Runes) != "日" || w.Width != 2 {
			t.Fatalf("wide cell = %q width %d, want 日 width 2", string(w.Runes), w.Width)
		}
		if sp := f.CellAt(1, 0); len(sp.Runes) != 0 || sp.Width != 0 {
			t.Fatalf("spacer cell = %q width %d, want empty width 0", string(sp.Runes), sp.Width)
		}
		if a := f.CellAt(2, 0); string(a.Runes) != "a" {
			t.Fatalf("cell after spacer = %q, want a", string(a.Runes))
		}
		if got := f.RowText(0); got != "日a" {
			t.Fatalf("RowText = %q, want 日a", got)
		}
		if f.Cursor.X != 3 {
			t.Fatalf("cursor after wide = %d, want 3", f.Cursor.X)
		}
	})

	t.Run("kitty_graphics_roundtrip_raw", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		pix := []byte{
			1, 2, 3, 255, 4, 5, 6, 255, 7, 8, 9, 255, 10, 11, 12, 255, // row 0
			13, 14, 15, 255, 16, 17, 18, 255, 19, 20, 21, 255, 22, 23, 24, 255, // row 1
		}
		writeKitty(t, e, fmt.Sprintf("a=T,f=32,s=4,v=2,i=7,q=2;%s",
			base64.StdEncoding.EncodeToString(pix)))

		f := snapshot(t, e)
		if len(f.Graphics.Placements) != 1 {
			t.Fatalf("placements = %d, want 1 (%+v)", len(f.Graphics.Placements), f.Graphics)
		}
		p := f.Graphics.Placements[0]
		if p.ImageID != 7 || p.Virtual {
			t.Fatalf("placement = %+v", p)
		}
		if p.Col != 0 || p.Row != 0 || p.PixelW != 4 || p.PixelH != 2 || p.SrcW != 4 || p.SrcH != 2 {
			t.Fatalf("geometry = %+v", p)
		}
		if f.Graphics.Generation == 0 {
			t.Fatal("generation must be nonzero after a transmit")
		}

		img, err := e.ImagePixels(7)
		if err != nil {
			t.Fatalf("ImagePixels: %v", err)
		}
		if img.W != 4 || img.H != 2 || !bytes.Equal(img.Pix, pix) {
			t.Fatalf("pixels = %dx%d %v", img.W, img.H, img.Pix)
		}
	})

	t.Run("kitty_graphics_png_straight_alpha", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
		copy(src.Pix, []byte{255, 0, 0, 255 /* opaque red */, 0, 128, 255, 128 /* translucent */})
		var buf bytes.Buffer
		if err := png.Encode(&buf, src); err != nil {
			t.Fatal(err)
		}
		writeKitty(t, e, fmt.Sprintf("a=T,f=100,i=9,q=2;%s",
			base64.StdEncoding.EncodeToString(buf.Bytes())))

		img, err := e.ImagePixels(9)
		if err != nil {
			t.Fatalf("ImagePixels: %v", err)
		}
		if img.W != 2 || img.H != 1 {
			t.Fatalf("decoded size = %dx%d", img.W, img.H)
		}
		if !bytes.Equal(img.Pix, src.Pix) {
			t.Fatalf("straight-alpha roundtrip broken: %v != %v", img.Pix, src.Pix)
		}
	})

	t.Run("generation_stamps", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		writeKitty(t, e, "a=T,f=32,s=1,v=1,i=1,q=2;"+
			base64.StdEncoding.EncodeToString([]byte{9, 9, 9, 255}))
		f := snapshot(t, e)
		gen1 := f.Graphics.Generation
		mustWrite(t, e, "text that leaves images untouched")
		f2 := snapshot(t, e)
		if f2.Graphics.Generation != gen1 {
			t.Fatalf("generation changed without graphics mutation: %d → %d", gen1, f2.Graphics.Generation)
		}
		writeKitty(t, e, "a=T,f=32,s=1,v=1,i=2,q=2;"+
			base64.StdEncoding.EncodeToString([]byte{1, 1, 1, 255}))
		f3 := snapshot(t, e)
		if f3.Graphics.Generation <= gen1 {
			t.Fatalf("generation must advance on new transmit: %d → %d", gen1, f3.Graphics.Generation)
		}
	})

	t.Run("key_encoder_kitty_csi_u", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		legacy, err := e.EncodeKey(vtengine.KeyEvent{Key: key.Named(key.NameEnter)})
		if err != nil || string(legacy) != "\r" {
			t.Fatalf("legacy Enter = %q, %v — want \\r", legacy, err)
		}
		// The app enables the kitty keyboard protocol (push flags=1).
		mustWrite(t, e, "\x1b[>1u")
		enhanced, err := e.EncodeKey(vtengine.KeyEvent{Key: key.Named(key.NameEscape)})
		if err != nil {
			t.Fatalf("enhanced Escape: %v", err)
		}
		if !regexp.MustCompile(`^\x1b\[27(;\d+)?u$`).Match(enhanced) {
			t.Fatalf("enhanced Escape = %q, want CSI 27 u encoding", enhanced)
		}
		// The app pops the flags; encoding returns to legacy.
		mustWrite(t, e, "\x1b[<u")
		back, err := e.EncodeKey(vtengine.KeyEvent{Key: key.Named(key.NameEscape)})
		if err != nil || string(back) != "\x1b" {
			t.Fatalf("post-pop Escape = %q, %v — want ESC", back, err)
		}
	})

	t.Run("capability_query_responses", func(t *testing.T) {
		opts := defaultOpts()
		var responses bytes.Buffer
		opts.Responses = &responses
		e := factory(t, opts)
		defer func() { _ = e.Close() }()

		mustWrite(t, e, "\x1b[c") // DA1: primary device attributes query
		if !bytes.HasPrefix(responses.Bytes(), []byte("\x1b[?")) {
			t.Fatalf("DA1 response = %q, want CSI ? … c", responses.Bytes())
		}
		responses.Reset()

		// kitty graphics query (a=q): how yazi probes for support.
		writeKitty(t, e, "i=31,s=1,v=1,a=q;"+
			base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 255}))
		if got := responses.String(); !bytes.Contains([]byte(got), []byte("\x1b_G")) ||
			!bytes.Contains([]byte(got), []byte("OK")) {
			t.Fatalf("kitty a=q response = %q, want APC …OK", got)
		}
	})
}

// writeKitty wraps a kitty graphics command in an APC sequence.
func writeKitty(t *testing.T, e vtengine.Engine, cmd string) {
	t.Helper()
	mustWrite(t, e, "\x1b_G"+cmd+"\x1b\\")
}
