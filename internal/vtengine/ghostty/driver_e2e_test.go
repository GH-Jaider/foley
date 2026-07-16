//go:build ghosttyvt

package ghostty_test

import (
	"context"
	"image"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/driver"
	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/ghostty"
	"github.com/GH-Jaider/foley/key"
)

type e2eSink struct {
	frames []time.Duration
	bounds image.Rectangle
}

func (s *e2eSink) Add(img *image.RGBA, d time.Duration) error {
	s.frames = append(s.frames, d)
	s.bounds = img.Bounds()
	return nil
}

func (s *e2eSink) Still(string, *image.RGBA) error { return nil }

// TestDriverRealEngineTypeAndRender composes the whole M7 stack for the
// first time: driver keystrokes encoded by the REAL engine, a real shell
// on a real pty, settle, and rasterized frames. The tty line discipline
// echoes typed characters kernel-side (deterministically, regardless of
// when the shell wakes), and the zero-duration-drop rule makes the one
// genuine race (whether the shell's reply lands in Enter's settle or in
// the Wait) invisible in the emitted frames — asserted exactly below.
func TestDriverRealEngineTypeAndRender(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	ras, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	cellW, cellH := ras.LogicalCellSize()
	geo := vtengine.Geometry{Cols: 40, Rows: 4, CellW: cellW, CellH: cellH}

	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", `read line; printf 'got: %s' "$line"`},
		Size:    ptyx.Winsize{Cols: 40, Rows: 4, XPix: 40 * cellW, YPix: 4 * cellH},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	e, err := ghostty.New(vtengine.Options{Geometry: geo, Responses: p})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

	var renders []string
	sink := &e2eSink{}
	d, err := driver.New(driver.Options{
		Engine:    e,
		Transport: p,
		Render: func(f *vtengine.Frame, dst *image.RGBA) (*image.RGBA, error) {
			renders = append(renders, f.Text())
			return ras.Render(f, e, dst)
		},
		Sink: sink,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := d.Type(ctx, "hola", 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := d.Press(ctx, key.Key{Name: key.NameEnter}, 0); err != nil {
		t.Fatal(err)
	}
	if err := d.WaitText(ctx, regexp.MustCompile(`got: hola`), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := d.Sleep(ctx, 500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := d.Finish(); err != nil {
		t.Fatal(err)
	}

	wantDurs := []time.Duration{
		30 * time.Millisecond, 30 * time.Millisecond,
		30 * time.Millisecond, 30 * time.Millisecond,
		500 * time.Millisecond,
	}
	if len(sink.frames) != len(wantDurs) {
		t.Fatalf("emitted %d frames (%v), want %d", len(sink.frames), sink.frames, len(wantDurs))
	}
	for i, want := range wantDurs {
		if sink.frames[i] != want {
			t.Fatalf("frame %d duration = %v, want %v (all: %v)", i, sink.frames[i], want, sink.frames)
		}
	}
	for i, want := range []string{"h", "ho", "hol", "hola"} {
		if renders[i] != want {
			t.Fatalf("render %d = %q, want %q", i, renders[i], want)
		}
	}
	if last := renders[len(renders)-1]; last != "hola\ngot: hola" {
		t.Fatalf("final render = %q", last)
	}
	// Output pixels follow CellSize (the scaled cell may be odd, e.g.
	// 19px, which integer-halved LogicalCellSize cannot reconstruct).
	scaledW, scaledH := ras.CellSize()
	if want := image.Rect(0, 0, 40*scaledW, 4*scaledH); sink.bounds != want {
		t.Fatalf("frame bounds = %v, want %v", sink.bounds, want)
	}
}

// rtTextRecorder is a RenderFunc + Sink pair for the realtime loop: each
// render goes to a fresh tagged image so Add can log the exact text the
// sink received, mutex-guarded because the loop goroutine writes it.
type rtTextRecorder struct {
	mu     sync.Mutex
	texts  map[*image.RGBA]string
	frames []string
}

func newRTTextRecorder() *rtTextRecorder {
	return &rtTextRecorder{texts: make(map[*image.RGBA]string)}
}

func (r *rtTextRecorder) render(f *vtengine.Frame, _ *image.RGBA) (*image.RGBA, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	r.texts[img] = f.Text()
	return img, nil
}

func (r *rtTextRecorder) Add(img *image.RGBA, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, r.texts[img])
	return nil
}

func (r *rtTextRecorder) Still(string, *image.RGBA) error { return nil }

func (r *rtTextRecorder) snap() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.frames...)
}

// TestRealtimeDriverRealEngine runs the wall-clock loop against the real
// engine and a real shell: keystrokes encoded by ghostty, echo and output
// sampled as they arrive. Content-only assertions.
func TestRealtimeDriverRealEngine(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", `read line; printf 'got: %s' "$line"`},
		Size:    ptyx.Winsize{Cols: 40, Rows: 4, XPix: 320, YPix: 64},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	e, err := ghostty.New(vtengine.Options{
		Geometry:  vtengine.Geometry{Cols: 40, Rows: 4, CellW: 8, CellH: 16},
		Responses: p,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

	rec := newRTTextRecorder()
	d, err := driver.NewRealtime(driver.RealtimeOptions{
		Engine: e, Transport: p, Render: rec.render, Sink: rec, FPS: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := d.Type(ctx, "hi", 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := d.Press(ctx, key.Key{Name: key.NameEnter}, 0); err != nil {
		t.Fatal(err)
	}
	if err := d.WaitText(ctx, regexp.MustCompile(`got: hi`), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := d.Finish(); err != nil {
		t.Fatal(err)
	}
	frames := rec.snap()
	if len(frames) == 0 {
		t.Fatal("realtime run emitted no frames")
	}
	if last := frames[len(frames)-1]; last != "hi\ngot: hi" {
		t.Fatalf("final frame = %q (all: %q)", last, frames)
	}
}

// TestDriverPumpsEngineResponses proves the Responses pump end to end:
// the app queries DA1 and BLOCKS reading the reply — it can only print
// PUMPED if the driver fed its query to the engine and the engine's
// answer traveled back through the pty. Without the pump this times out
// (with a screen dump) instead of passing by luck.
func TestDriverPumpsEngineResponses(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", `stty raw; printf '\033[c'; head -c 4 >/dev/null; printf 'PUMPED'`},
		Size:    ptyx.Winsize{Cols: 40, Rows: 4, XPix: 320, YPix: 64},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	e, err := ghostty.New(vtengine.Options{
		Geometry:  vtengine.Geometry{Cols: 40, Rows: 4, CellW: 8, CellH: 16},
		Responses: p,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

	d, err := driver.New(driver.Options{
		Engine:    e,
		Transport: p,
		Render: func(_ *vtengine.Frame, dst *image.RGBA) (*image.RGBA, error) {
			if dst == nil {
				dst = image.NewRGBA(image.Rect(0, 0, 1, 1))
			}
			return dst, nil
		},
		Sink: &e2eSink{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.WaitText(context.Background(), regexp.MustCompile(`PUMPED`), 5*time.Second); err != nil {
		t.Fatal(err)
	}
}
