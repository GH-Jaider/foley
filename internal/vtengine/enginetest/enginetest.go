package enginetest

import (
	"errors"
	"testing"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/key"
)

// Factory constructs a fresh engine for one test. Each subtest gets its
// own instance; the suite closes it.
type Factory func(t *testing.T, opts vtengine.Options) vtengine.Engine

func defaultOpts() vtengine.Options {
	return vtengine.Options{
		Geometry:          vtengine.Geometry{Cols: 20, Rows: 4, CellW: 8, CellH: 16},
		KittyStorageLimit: 8 << 20,
	}
}

// RunBasic exercises the contract surface every engine — including the
// puppet fake — must honor: write/snapshot round-trips for plain text,
// cursor motion for CR/LF, resize, frame reuse and closed-engine errors.
func RunBasic(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("write_snapshot_text", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		mustWrite(t, e, "hola foley")
		f := snapshot(t, e)
		if got := f.RowText(0); got != "hola foley" {
			t.Fatalf("RowText(0) = %q, want %q", got, "hola foley")
		}
		if !f.Dirty {
			t.Fatal("first snapshot must report Dirty")
		}
	})

	t.Run("crlf_moves_cursor", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		mustWrite(t, e, "uno\r\ndos")
		f := snapshot(t, e)
		if got := f.RowText(1); got != "dos" {
			t.Fatalf("RowText(1) = %q, want %q", got, "dos")
		}
		if f.Cursor.Y != 1 || f.Cursor.X != 3 {
			t.Fatalf("cursor = (%d,%d), want (3,1)", f.Cursor.X, f.Cursor.Y)
		}
	})

	t.Run("screen_text", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		mustWrite(t, e, "a\r\nb")
		f := snapshot(t, e)
		if got := f.Text(); got != "a\nb" {
			t.Fatalf("Text() = %q, want %q", got, "a\nb")
		}
	})

	t.Run("resize_changes_geometry", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		g := vtengine.Geometry{Cols: 33, Rows: 7, CellW: 10, CellH: 21}
		if err := e.Resize(g); err != nil {
			t.Fatalf("Resize: %v", err)
		}
		f := snapshot(t, e)
		if f.Geometry != g {
			t.Fatalf("Geometry = %+v, want %+v", f.Geometry, g)
		}
		if len(f.Cells) != g.Cols*g.Rows {
			t.Fatalf("len(Cells) = %d, want %d", len(f.Cells), g.Cols*g.Rows)
		}
	})

	t.Run("frame_reuse", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		mustWrite(t, e, "x")
		var f vtengine.Frame
		if err := e.Snapshot(&f); err != nil {
			t.Fatalf("Snapshot 1: %v", err)
		}
		mustWrite(t, e, "y")
		if err := e.Snapshot(&f); err != nil {
			t.Fatalf("Snapshot 2 (reused frame): %v", err)
		}
		if got := f.RowText(0); got != "xy" {
			t.Fatalf("RowText(0) after reuse = %q, want %q", got, "xy")
		}
	})

	t.Run("initial_colors_applied", func(t *testing.T) {
		opts := defaultOpts()
		colors := vtengine.Colors{
			FG: vtengine.RGB{R: 1, G: 2, B: 3},
			BG: vtengine.RGB{R: 9, G: 8, B: 7},
		}
		colors.Palette[42] = vtengine.RGB{R: 4, G: 2, B: 42}
		opts.Colors = &colors
		e := factory(t, opts)
		defer func() { _ = e.Close() }()
		f := snapshot(t, e)
		if f.Colors.FG != colors.FG || f.Colors.BG != colors.BG {
			t.Fatalf("colors = fg %+v bg %+v, want fg %+v bg %+v",
				f.Colors.FG, f.Colors.BG, colors.FG, colors.BG)
		}
		if f.Colors.Palette[42] != colors.Palette[42] {
			t.Fatalf("palette[42] = %+v, want %+v", f.Colors.Palette[42], colors.Palette[42])
		}
	})

	t.Run("encode_key_basics", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		got, err := e.EncodeKey(vtengine.KeyEvent{Key: key.RuneKey('a')})
		if err != nil || len(got) == 0 {
			t.Fatalf("EncodeKey('a') = %q, %v — want non-empty, nil", got, err)
		}
		got, err = e.EncodeKey(vtengine.KeyEvent{Key: key.Named(key.NameEnter)})
		if err != nil || len(got) == 0 {
			t.Fatalf("EncodeKey(Enter) = %q, %v — want non-empty, nil", got, err)
		}
	})

	t.Run("unknown_image", func(t *testing.T) {
		e := factory(t, defaultOpts())
		defer func() { _ = e.Close() }()
		if _, err := e.ImagePixels(424242); !errors.Is(err, vtengine.ErrNoImage) {
			t.Fatalf("ImagePixels(unknown) = %v, want ErrNoImage", err)
		}
	})

	t.Run("closed_engine_errors", func(t *testing.T) {
		e := factory(t, defaultOpts())
		if err := e.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := e.Close(); err != nil {
			t.Fatalf("Close must be idempotent, got %v", err)
		}
		if _, err := e.Write([]byte("x")); !errors.Is(err, vtengine.ErrClosed) {
			t.Fatalf("Write after Close = %v, want ErrClosed", err)
		}
		var f vtengine.Frame
		if err := e.Snapshot(&f); !errors.Is(err, vtengine.ErrClosed) {
			t.Fatalf("Snapshot after Close = %v, want ErrClosed", err)
		}
	})
}

// RunFull runs RunBasic plus full VT conformance: SGR styles, palette and
// true color, wide graphemes, scrolling, kitty graphics round-trips
// (transmit → placement geometry → pixels → generation stamps) and
// keyboard-protocol-aware key encoding. Real engines (M3: ghostty) must
// pass it unchanged; the cases land with the first real engine so each
// assertion is written against observed protocol behavior, never guessed.
func RunFull(t *testing.T, factory Factory) {
	t.Helper()
	RunBasic(t, factory)
	// M3: sgr_styles, true_color_and_palette, wide_graphemes, scrollback,
	// kitty_graphics_roundtrip, generation_stamps, key_encoder_legacy,
	// key_encoder_kitty_csi_u, capability_query_responses (a=q reaches
	// Options.Responses), image_pixels_straight_alpha.
}

func mustWrite(t *testing.T, e vtengine.Engine, s string) {
	t.Helper()
	if _, err := e.Write([]byte(s)); err != nil {
		t.Fatalf("Write(%q): %v", s, err)
	}
}

func snapshot(t *testing.T, e vtengine.Engine) *vtengine.Frame {
	t.Helper()
	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return &f
}
