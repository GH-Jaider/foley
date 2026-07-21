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

	t.Run("osc_title", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		f := snapshot(t, e)
		if f.Title != "" {
			t.Fatalf("title before any OSC = %q, want empty", f.Title)
		}
		mustWrite(t, e, "\x1b]2;vim retry.go\x1b\\")
		f = snapshot(t, e)
		if f.Title != "vim retry.go" {
			t.Fatalf("OSC 2 title = %q, want vim retry.go", f.Title)
		}
		// A PURE title change (no cell touched) must still dirty the
		// frame — chrome following the title cannot skip it.
		snapshot(t, e) // drain dirty
		mustWrite(t, e, "\x1b]0;tmux\x07")
		f = snapshot(t, e)
		if f.Title != "tmux" {
			t.Fatalf("OSC 0 title = %q, want tmux", f.Title)
		}
		if !f.Dirty {
			t.Fatal("a title change alone must dirty the frame")
		}
	})

	t.Run("kitty_graphics_scaled_placement_anchor", func(t *testing.T) {
		// A placement scaled by c= (columns) alone must still anchor at
		// the cursor — real previewers (lf's, icat) routinely send
		// c-only and let the aspect pick the rows. Found live: the lf
		// example's photo rendered at the grid origin.
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		pix := make([]byte, 4*4*4) // 4x4 RGBA
		for i := range pix {
			pix[i] = 0xff
		}
		payload := base64.StdEncoding.EncodeToString(pix)
		mustWrite(t, e, "\x1b[2;5H") // anchor at row 1, col 4 (0-based)
		writeKitty(t, e, fmt.Sprintf("a=T,f=32,s=4,v=4,i=11,c=2,q=2;%s", payload))
		f := snapshot(t, e)
		if len(f.Graphics.Placements) != 1 {
			t.Fatalf("placements = %d, want 1 (%+v)", len(f.Graphics.Placements), f.Graphics)
		}
		p := f.Graphics.Placements[0]
		if p.Col != 4 || p.Row != 1 {
			t.Fatalf("c-only placement anchored at (%d,%d), want the cursor (4,1)", p.Col, p.Row)
		}
		if p.PixelW == 0 || p.PixelH == 0 {
			t.Fatalf("c-only placement has no pixel geometry: %+v", p)
		}

		// The same contract for PNG data: the aspect math needs the
		// DECODED dimensions (the live failure was a PNG previewer).
		src := image.NewNRGBA(image.Rect(0, 0, 6, 6))
		for i := range src.Pix {
			src.Pix[i] = 0xaa
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, src); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, e, "\x1b[3;7H") // anchor at row 2, col 6 (0-based)
		writeKitty(t, e, fmt.Sprintf("a=T,f=100,i=12,c=2,q=2;%s",
			base64.StdEncoding.EncodeToString(buf.Bytes())))
		f = snapshot(t, e)
		var pngP *vtengine.Placement
		for i := range f.Graphics.Placements {
			if f.Graphics.Placements[i].ImageID == 12 {
				pngP = &f.Graphics.Placements[i]
			}
		}
		if pngP == nil {
			t.Fatalf("png c-only placement missing: %+v", f.Graphics)
		}
		if pngP.Col != 6 || pngP.Row != 2 {
			t.Fatalf("png c-only placement anchored at (%d,%d), want the cursor (6,2)", pngP.Col, pngP.Row)
		}
		if pngP.PixelW == 0 || pngP.PixelH == 0 {
			t.Fatalf("png c-only placement has no pixel geometry: %+v", pngP)
		}

		// A LARGE payload arrives from a real pty in many read chunks —
		// the APC crosses hundreds of Write calls. The anchor must not
		// drift (the live failure was a 500KB photo previewer).
		big := image.NewNRGBA(image.Rect(0, 0, 300, 300))
		for i := range big.Pix {
			big.Pix[i] = byte(i * 7)
		}
		buf.Reset()
		if err := png.Encode(&buf, big); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, e, "\x1b[2;3H") // anchor at row 1, col 2 (0-based)
		apc := fmt.Sprintf("\x1b_Ga=T,f=100,i=13,c=2,q=2;%s\x1b\\",
			base64.StdEncoding.EncodeToString(buf.Bytes()))
		for len(apc) > 0 {
			n := min(1024, len(apc))
			mustWrite(t, e, apc[:n])
			apc = apc[n:]
		}
		f = snapshot(t, e)
		var bigP *vtengine.Placement
		for i := range f.Graphics.Placements {
			if f.Graphics.Placements[i].ImageID == 13 {
				bigP = &f.Graphics.Placements[i]
			}
		}
		if bigP == nil {
			t.Fatalf("big chunked placement missing: %+v", f.Graphics)
		}
		if bigP.Col != 2 || bigP.Row != 1 {
			t.Fatalf("big chunked placement anchored at (%d,%d), want the cursor (2,1)", bigP.Col, bigP.Row)
		}

		// The previewer idiom: DECSC, jump, transmit, DECRC — one write.
		// The anchor is the cursor AT TRANSMISSION, not the restored
		// one (found live: lf's photo anchored at the pre-jump cell).
		writeKitty(t, e, "a=d,d=A") // clear the stage
		mustWrite(t, e, "\x1b[1;1H")
		mustWrite(t, e, fmt.Sprintf("\x1b7\x1b[3;9H\x1b_Ga=T,f=32,s=4,v=4,i=14,c=2,q=2;%s\x1b\\\x1b8", payload))
		f = snapshot(t, e)
		var wrapped *vtengine.Placement
		for i := range f.Graphics.Placements {
			if f.Graphics.Placements[i].ImageID == 14 {
				wrapped = &f.Graphics.Placements[i]
			}
		}
		if wrapped == nil {
			t.Fatalf("DECSC-wrapped placement missing: %+v", f.Graphics)
		}
		if wrapped.Col != 8 || wrapped.Row != 2 {
			t.Fatalf("DECSC-wrapped placement anchored at (%d,%d), want the jumped cursor (8,2)", wrapped.Col, wrapped.Row)
		}

		// CHARACTERIZATION, not aspiration: a placement whose derived
		// rows OVERFLOW the space below the anchor gets RELOCATED by
		// libghostty — both axes, toward the origin — instead of
		// kitty's scroll-to-fit (rows shift, column holds). Found live
		// (the lf photo pinned to the top-left); reproduced across the
		// whole pipeline. Pinned here so a lib bump that changes the
		// behavior is NOTICED; the app-side answer stays "fit your
		// placements" (c AND r), which real previewers should do
		// anyway. Upstream question for the pin.
		writeKitty(t, e, "a=d,d=A")
		mustWrite(t, e, "\x1b[3;16H") // anchor (15,2); c=10 → ~5 rows > the 2 below
		writeKitty(t, e, fmt.Sprintf("a=T,f=32,s=4,v=4,i=15,c=10,q=2;%s", payload))
		f = snapshot(t, e)
		var over *vtengine.Placement
		for i := range f.Graphics.Placements {
			if f.Graphics.Placements[i].ImageID == 15 {
				over = &f.Graphics.Placements[i]
			}
		}
		if over == nil {
			t.Fatalf("overflowing placement missing: %+v", f.Graphics)
		}
		if over.Col == 15 && over.Row == 2 {
			t.Fatal("the lib now anchors overflowing placements at the cursor — kitty semantics arrived with a pin bump: drop this characterization and the fit-workarounds it documents")
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
		// A graphics mutation carries no cell damage, but it IS a visible
		// change: the frame must come out dirty or a realtime recording
		// freezes on animations that only retransmit images (found live
		// with tenten's pixel mode).
		if !f3.Dirty {
			t.Fatal("graphics-only mutation must dirty the frame")
		}
		f4 := snapshot(t, e)
		if f4.Dirty {
			t.Fatal("quiet snapshot after a graphics change must be clean")
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

	// The tape DSL's Ctrl/Alt chords depend on these encodings — every
	// migrated tape has a Ctrl+C. Default mode is xterm/xterm.js parity
	// (what a VHS tape meant): ctrl-letter folds into C0, Shift folds out
	// of Ctrl chords, alt prefixes ESC, and keys with no legacy form
	// degrade to their unmodified selves instead of CSI-27 noise.
	t.Run("key_encoder_modifiers_xterm_parity", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		cases := []struct {
			name string
			ev   vtengine.KeyEvent
			want string
		}{
			{"ctrl_c", vtengine.KeyEvent{Key: key.RuneKey('c').With(key.ModCtrl)}, "\x03"},
			{"ctrl_l", vtengine.KeyEvent{Key: key.RuneKey('l').With(key.ModCtrl)}, "\x0c"},
			{"alt_x", vtengine.KeyEvent{Key: key.RuneKey('x').With(key.ModAlt)}, "\x1bx"},
			{"ctrl_shift_c_folds", vtengine.KeyEvent{Key: key.RuneKey('c').With(key.ModCtrl | key.ModShift)}, "\x03"},
			{"ctrl_left", vtengine.KeyEvent{Key: key.Named(key.NameLeft).With(key.ModCtrl)}, "\x1b[1;5D"},
			{"shift_tab", vtengine.KeyEvent{Key: key.Named(key.NameTab).With(key.ModShift)}, "\x1b[Z"},
			{"ctrl_enter_degrades", vtengine.KeyEvent{Key: key.Named(key.NameEnter).With(key.ModCtrl)}, "\r"},
			{"shift_a_text", vtengine.KeyEvent{Key: key.RuneKey('A').With(key.ModShift)}, "A"},
			// Space IS text — a real tape found it encoding empty.
			{"space", vtengine.KeyEvent{Key: key.Named(key.NameSpace)}, " "},
			{"ctrl_space_nul", vtengine.KeyEvent{Key: key.Named(key.NameSpace).With(key.ModCtrl)}, "\x00"},
			{"alt_space", vtengine.KeyEvent{Key: key.Named(key.NameSpace).With(key.ModAlt)}, "\x1b "},
		}
		for _, c := range cases {
			got, err := e.EncodeKey(c.ev)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if string(got) != c.want {
				t.Fatalf("%s = %q, want %q", c.name, got, c.want)
			}
		}
	})

	// Opting into ModifyOtherKeys keeps the modern CSI-27 forms.
	t.Run("key_encoder_modify_other_keys", func(t *testing.T) {
		opts := defaultOpts()
		opts.ModifyOtherKeys = true
		e := factory(t, opts)
		defer func() { _ = e.Close() }()
		cases := []struct {
			name string
			ev   vtengine.KeyEvent
			want string
		}{
			{"ctrl_enter", vtengine.KeyEvent{Key: key.Named(key.NameEnter).With(key.ModCtrl)}, "\x1b[27;5;13~"},
			{"shift_tab", vtengine.KeyEvent{Key: key.Named(key.NameTab).With(key.ModShift)}, "\x1b[27;2;9~"},
			{"ctrl_c_still_c0", vtengine.KeyEvent{Key: key.RuneKey('c').With(key.ModCtrl)}, "\x03"},
		}
		for _, c := range cases {
			got, err := e.EncodeKey(c.ev)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if string(got) != c.want {
				t.Fatalf("%s = %q, want %q", c.name, got, c.want)
			}
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

	t.Run("geometry_query_responses", func(t *testing.T) {
		// XTWINOPS reports and XTGETTCAP: the startup interrogation of
		// modern TUIs (opencode-class). An engine must answer them —
		// pixel geometry from its Geometry, identity from the pinned
		// terminfo — or a deterministic take reads the app's reply
		// timeouts as silence and moves on before it draws (ADR-025).
		opts := defaultOpts() // 20×4 cells of 8×16 px
		opts.Colors = &vtengine.Colors{
			FG: vtengine.RGB{R: 200, G: 200, B: 200},
			BG: vtengine.RGB{R: 30, G: 30, B: 46}, // dark: scheme report = 1
		}
		var responses bytes.Buffer
		opts.Responses = &responses
		e := factory(t, opts)
		defer func() { _ = e.Close() }()

		cases := []struct{ name, query, want string }{
			{"window_state_11t", "\x1b[11t", "\x1b[1t"},
			{"text_area_px_14t", "\x1b[14t", "\x1b[4;64;160t"},
			{"screen_px_15t", "\x1b[15t", "\x1b[5;64;160t"},
			{"cell_px_16t", "\x1b[16t", "\x1b[6;16;8t"},
			{"text_area_cells_18t", "\x1b[18t", "\x1b[8;4;20t"},
			{"screen_cells_19t", "\x1b[19t", "\x1b[9;4;20t"},
			// XTGETTCAP: TN answers the declared identity and Tc the
			// truecolor flag, both from the pinned terminfo story;
			// unknown capabilities get an immediate negative — a prompt
			// "no" ends a reply timeout just as well as a "yes". Same
			// doctrine for DECRQSS (vim's startup probes) and the
			// color-scheme report neovim-era apps use to pick a theme.
			{
				"xtgettcap_tn", "\x1bP+q544e\x1b\\",
				"\x1bP1+r544e=787465726d2d67686f73747479\x1b\\",
			},
			{"xtgettcap_truecolor_flag", "\x1bP+q5463\x1b\\", "\x1bP1+r5463\x1b\\"},
			{"xtgettcap_unknown", "\x1bP+q6e6f7065\x1b\\", "\x1bP0+r\x1b\\"},
			{"decrqss_negative", "\x1bP$qm\x1b\\", "\x1bP0$r\x1b\\"},
			{"color_scheme_996", "\x1b[?996n", "\x1b[?997;1n"},
		}
		for _, c := range cases {
			responses.Reset()
			mustWrite(t, e, c.query)
			if got := responses.String(); got != c.want {
				t.Fatalf("%s response = %q, want %q", c.name, got, c.want)
			}
		}

		// Geometry answers must track Resize.
		if err := e.Resize(vtengine.Geometry{Cols: 10, Rows: 2, CellW: 10, CellH: 20}); err != nil {
			t.Fatalf("resize: %v", err)
		}
		responses.Reset()
		mustWrite(t, e, "\x1b[14t")
		if got := responses.String(); got != "\x1b[4;40;100t" {
			t.Fatalf("14t after resize = %q, want %q", got, "\x1b[4;40;100t")
		}
	})
}

// writeKitty wraps a kitty graphics command in an APC sequence.
func writeKitty(t *testing.T, e vtengine.Engine, cmd string) {
	t.Helper()
	mustWrite(t, e, "\x1b_G"+cmd+"\x1b\\")
}
