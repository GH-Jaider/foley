//go:build ghosttyvt

package ghostty_test

import (
	"context"
	"image"
	"image/gif"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/driver"
	"github.com/GH-Jaider/foley/internal/encode"
	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/ghostty"
	"github.com/GH-Jaider/foley/key"
)

// TestFirstRecording is foley's whole thesis in one test: a real shell on
// a real pty, the embedded engine, deterministic virtual time, the
// rasterizer and the encode pipeline — producing an actual animated GIF
// with typed input, colors, ligature-bearing text and an emoji. No
// terminal window was involved. Set FOLEY_DEMO_OUT to keep the GIF.
func TestFirstRecording(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (CI runners ship it)")
	pack, err := fontpack.Load(filepath.Join("..", "..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")

	ras, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	cellW, cellH := ras.LogicalCellSize()
	geo := vtengine.Geometry{Cols: 48, Rows: 6, CellW: cellW, CellH: cellH}

	script := `printf '\033[1;38;2;137;180;250mfoley\033[0m \033[38;2;108;112;134m~ demo\033[0m\r\n'
read line
printf '\033[38;2;166;227;161mok:\033[0m %s\r\n' "$line"
printf 'ligaduras -> => != listas \360\237\232\200\r\n'
read hold`
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", script},
		Size:    ptyx.Winsize{Cols: 48, Rows: 6, XPix: 48 * cellW, YPix: 6 * cellH},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	e, err := ghostty.New(vtengine.Options{
		Geometry: geo,
		Colors: &vtengine.Colors{
			FG: vtengine.RGB{R: 0xcd, G: 0xd6, B: 0xf4},
			BG: vtengine.RGB{R: 0x1e, G: 0x1e, B: 0x2e},
		},
		Responses: p,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

	framesDir := t.TempDir()
	sink, err := encode.NewPNGSink(encode.PNGSinkOptions{Dir: framesDir})
	if err != nil {
		t.Fatal(err)
	}
	d, err := driver.New(driver.Options{
		Engine:    e,
		Transport: p,
		Render: func(f *vtengine.Frame, dst *image.RGBA) (*image.RGBA, error) {
			return ras.Render(f, e, dst)
		},
		Sink: sink,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := d.Sleep(ctx, 600*time.Millisecond); err != nil { // banner on screen
		t.Fatal(err)
	}
	if err := d.Type(ctx, "hola foley", 60*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := d.Press(ctx, key.Key{Name: key.NameEnter}, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := d.WaitText(ctx, regexp.MustCompile(`listas`), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := d.Sleep(ctx, 1200*time.Millisecond); err != nil { // let the result breathe
		t.Fatal(err)
	}
	if err := d.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	gifPath := filepath.Join(framesDir, "demo.gif")
	if err := encode.GIF(ctx, framesDir, gifPath); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(gifPath) //nolint:gosec // path built from TempDir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != sink.Frames() {
		t.Fatalf("gif has %d frames, sink emitted %d", len(g.Image), sink.Frames())
	}
	if len(g.Image) < 12 { // banner + 10 keystrokes + result
		t.Fatalf("recording suspiciously short: %d frames", len(g.Image))
	}
	// EXACT timing, stall-proof: banner and per-keystroke delays are
	// invariant; the tail after typing may legitimately split into 1 or 2
	// states (whether the shell's reply lands in Enter's settle or in the
	// Wait), so it is asserted by SUM — 100ms Enter + 1200ms hold = 130cs
	// either way, with the closing hold dominating.
	delays := g.Delay
	if delays[0] != 60 {
		t.Fatalf("banner delay = %dcs, want 60 (all: %v)", delays[0], delays)
	}
	for i := 1; i <= 10; i++ {
		if delays[i] != 6 {
			t.Fatalf("keystroke %d delay = %dcs, want 6 (all: %v)", i, delays[i], delays)
		}
	}
	tail := 0
	for _, d := range delays[11:] {
		tail += d
	}
	if tail != 130 {
		t.Fatalf("tail delays sum %dcs, want 130 (all: %v)", tail, delays)
	}
	if last := delays[len(delays)-1]; last < 120 {
		t.Fatalf("final hold = %dcs, want >= 120 (all: %v)", last, delays)
	}
	// The emoji's hues must SURVIVE to actual GIF pixels: nothing else in
	// this demo is red or yellow, so finding them in any frame proves the
	// palette kept rare-but-distinct colors (palettegen stats_mode=diff
	// starved them once — the rocket came out gray-blue).
	var red, yellow bool
	for _, frame := range g.Image {
		for i := range frame.Pix {
			r, gg, b, _ := frame.Palette[frame.Pix[i]].RGBA()
			r8, g8, b8 := int(r>>8), int(gg>>8), int(b>>8)
			if r8 > 150 && r8 > g8+60 && r8 > b8+60 {
				red = true
			}
			if r8 > 170 && g8 > 130 && b8 < 110 {
				yellow = true
			}
		}
		if red && yellow {
			break
		}
	}
	if !red || !yellow {
		t.Fatalf("emoji hues lost to the GIF palette (red=%v yellow=%v)", red, yellow)
	}

	if out := os.Getenv("FOLEY_DEMO_OUT"); out != "" {
		data, err := os.ReadFile(gifPath) //nolint:gosec // path built from TempDir
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, data, 0o644); err != nil { //nolint:gosec // user-requested artifact
			t.Fatal(err)
		}
		t.Logf("demo GIF kept at %s (%d frames)", out, len(g.Image))
	}
}
