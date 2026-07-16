package driver_test

import (
	"context"
	"errors"
	"image"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/driver"
	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
	"github.com/GH-Jaider/foley/key"
)

// transport is a synthetic Transport. With echo on, Write pushes the bytes
// back as a chunk before returning — a pty in echo mode, minus the pty.
// The mutex makes `written` pollable while a Realtime loop is writing.
type transport struct {
	ch   chan ptyx.Chunk
	echo bool

	mu      sync.Mutex
	written []byte
}

func newTransport(echo bool) *transport {
	return &transport{ch: make(chan ptyx.Chunk, 64), echo: echo}
}

func (t *transport) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.written = append(t.written, p...)
	t.mu.Unlock()
	if t.echo {
		t.feed(string(p))
	}
	return len(p), nil
}

func (t *transport) snapWritten() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.written...)
}

func (t *transport) Chunks() <-chan ptyx.Chunk { return t.ch }

func (t *transport) feed(s string) {
	t.ch <- ptyx.Chunk{Data: []byte(s), Time: time.Time{}}
}

// recorder is RenderFunc + Sink in one: it renders each state to a fresh
// tagged image and logs exactly what the sink receives. The mutex makes
// it pollable from the test while a Realtime loop is writing.
type recorder struct {
	mu     sync.Mutex
	texts  map[*image.RGBA]string
	frames []frameRec
	stills []stillRec
}

type frameRec struct {
	text string
	dur  time.Duration
}

type stillRec struct {
	name string
	text string
}

func newRecorder() *recorder {
	return &recorder{texts: make(map[*image.RGBA]string)}
}

func (r *recorder) render(f *vtengine.Frame, _ *image.RGBA) (*image.RGBA, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	r.texts[img] = f.Text()
	return img, nil
}

func (r *recorder) Add(img *image.RGBA, d time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, frameRec{r.texts[img], d})
	return nil
}

func (r *recorder) Still(name string, img *image.RGBA) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stills = append(r.stills, stillRec{name, r.texts[img]})
	return nil
}

func (r *recorder) snapFrames() []frameRec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]frameRec(nil), r.frames...)
}

func (r *recorder) snapStills() []stillRec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]stillRec(nil), r.stills...)
}

type rig struct {
	d  *driver.Driver
	tr *transport
	r  *recorder
}

func newRig(t *testing.T, echo bool) *rig {
	t.Helper()
	e := fake.New(vtengine.Options{Geometry: vtengine.Geometry{Cols: 20, Rows: 4}})
	tr := newTransport(echo)
	r := newRecorder()
	d, err := driver.New(driver.Options{
		Engine:    e,
		Transport: tr,
		Render:    r.render,
		Sink:      r,
		// Tiny wall knobs: settles here are one-sided delays — the test
		// controls exactly what is in the channel, so short timers only
		// speed things up, they can never change the outcome.
		Settle: driver.SettleOptions{First: 2 * time.Millisecond, Quiet: 2 * time.Millisecond, Max: time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &rig{d: d, tr: tr, r: r}
}

func (g *rig) mustFrames(t *testing.T, want ...frameRec) {
	t.Helper()
	if err := g.d.Finish(); err != nil {
		t.Fatal(err)
	}
	if len(g.r.frames) != len(want) {
		t.Fatalf("frames = %+v, want %+v", g.r.frames, want)
	}
	for i := range want {
		if g.r.frames[i] != want[i] {
			t.Fatalf("frame %d = %+v, want %+v", i, g.r.frames[i], want[i])
		}
	}
}

func TestSleepCoalescesIntoOneFrame(t *testing.T) {
	g := newRig(t, false)
	ctx := context.Background()
	if err := g.d.Sleep(ctx, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Sleep(ctx, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if g.d.Now() != 3*time.Second {
		t.Fatalf("Now = %v", g.d.Now())
	}
	g.mustFrames(t, frameRec{"", 3 * time.Second})
}

func TestTypeEmitsOneFramePerKeystroke(t *testing.T) {
	g := newRig(t, true)
	if err := g.d.Type(context.Background(), "ab", 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	g.mustFrames(t,
		frameRec{"a", 50 * time.Millisecond},
		frameRec{"ab", 50 * time.Millisecond},
	)
}

func TestZeroPerKeyCollapsesToOneFrame(t *testing.T) {
	g := newRig(t, true)
	ctx := context.Background()
	if err := g.d.Type(ctx, "abc", 0); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Sleep(ctx, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	// The intermediate "a"/"ab" states were visible for zero time: no
	// frames for them, and the whole burst costs a single render.
	g.mustFrames(t, frameRec{"abc", 2 * time.Second})
}

func TestPendingOutputLandsDeterministically(t *testing.T) {
	g := newRig(t, false)
	g.tr.feed("$ ") // the app prompt arrived before the first action
	if err := g.d.Sleep(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	// The step's settle absorbed the prompt, so frame one shows it.
	g.mustFrames(t, frameRec{"$", time.Second})
}

func TestPressEncodesThroughEngine(t *testing.T) {
	g := newRig(t, false)
	if err := g.d.Press(context.Background(), key.Key{Name: key.NameEnter}, 0); err != nil {
		t.Fatal(err)
	}
	// The fake engine encodes Enter as \r; the app must receive exactly
	// that through the transport.
	if got := string(g.tr.snapWritten()); got != "\r" {
		t.Fatalf("transport received %q, want \\r", got)
	}
}

func TestWaitMatchesAndConsumesNoVirtualTime(t *testing.T) {
	g := newRig(t, false)
	go func() {
		time.Sleep(5 * time.Millisecond)
		g.tr.feed("READY")
	}()
	if err := g.d.WaitText(context.Background(), regexp.MustCompile(`READY`), time.Second); err != nil {
		t.Fatal(err)
	}
	if g.d.Now() != 0 {
		t.Fatalf("Wait consumed virtual time: %v", g.d.Now())
	}
	if err := g.d.Sleep(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	g.mustFrames(t, frameRec{"READY", time.Second})
}

// TestWaitDirtinessSurvivesToNextFrame guards the driver's most
// load-bearing subtlety: Wait's intermediate snapshots must NOT swallow
// the dirtiness that the next emitted frame has to reflect. Proven
// necessary by mutation: with `d.dirty = d.frame.Dirty` (no
// accumulation) this test fails — the second Sleep would extend the
// blank frame to 2s instead of emitting READY.
func TestWaitDirtinessSurvivesToNextFrame(t *testing.T) {
	g := newRig(t, false)
	ctx := context.Background()
	if err := g.d.Sleep(ctx, time.Second); err != nil { // pending: ("", 1s)
		t.Fatal(err)
	}
	g.tr.feed("READY")
	if err := g.d.WaitText(ctx, regexp.MustCompile(`READY`), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Sleep(ctx, time.Second); err != nil {
		t.Fatal(err)
	}
	g.mustFrames(t,
		frameRec{"", time.Second},
		frameRec{"READY", time.Second},
	)
}

func TestWaitTimeoutDumpsScreen(t *testing.T) {
	g := newRig(t, false)
	g.tr.feed("something else")
	err := g.d.WaitText(context.Background(), regexp.MustCompile(`NEVER`), 10*time.Millisecond)
	if !errors.Is(err, driver.ErrWaitTimeout) {
		t.Fatalf("err = %v, want ErrWaitTimeout", err)
	}
	if !strings.Contains(err.Error(), "something else") {
		t.Fatalf("timeout error lacks the screen dump: %v", err)
	}
}

func TestWaitInterruptedOnAppExit(t *testing.T) {
	g := newRig(t, false)
	close(g.tr.ch) // the app exited: the predicate can never match now
	err := g.d.WaitText(context.Background(), regexp.MustCompile(`NEVER`), time.Second)
	if !errors.Is(err, driver.ErrWaitInterrupted) {
		t.Fatalf("err = %v, want ErrWaitInterrupted", err)
	}
	if errors.Is(err, driver.ErrWaitTimeout) {
		t.Fatalf("an interruption must not classify as a timeout: %v", err)
	}
}

func TestHideShowTrimsEmittedTimeline(t *testing.T) {
	g := newRig(t, true)
	ctx := context.Background()
	if err := g.d.Type(ctx, "a", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Hide(); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Type(ctx, "b", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Show(); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Sleep(ctx, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if g.d.Now() != 40*time.Millisecond {
		t.Fatalf("virtual time must include the hidden span: %v", g.d.Now())
	}
	g.mustFrames(t,
		frameRec{"a", 10 * time.Millisecond},
		frameRec{"ab", 20 * time.Millisecond}, // hidden typing surfaces on Show
	)
}

func TestScreenshotWorksWhileHidden(t *testing.T) {
	g := newRig(t, true)
	ctx := context.Background()
	if err := g.d.Hide(); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Type(ctx, "secret", 0); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Screenshot("proof"); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Finish(); err != nil {
		t.Fatal(err)
	}
	if len(g.r.frames) != 0 {
		t.Fatalf("hidden run emitted frames: %+v", g.r.frames)
	}
	want := stillRec{"proof", "secret"}
	if len(g.r.stills) != 1 || g.r.stills[0] != want {
		t.Fatalf("stills = %+v, want [%+v]", g.r.stills, want)
	}
}

func TestSettleMaxCapsAStreamingApp(t *testing.T) {
	// Quiet(2s) can only fire after a 2s gap; the flooder never leaves
	// one on purpose, so returning well under 1s proves Max(50ms) cut
	// the settle. The 20x margin absorbs CI-runner scheduler stalls
	// (macos-15 has shown >100ms ones).
	e := fake.New(vtengine.Options{Geometry: vtengine.Geometry{Cols: 20, Rows: 4}})
	tr := newTransport(false)
	r := newRecorder()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case tr.ch <- ptyx.Chunk{Data: []byte("x")}:
			case <-stop:
				return
			}
		}
	}()
	d, err := driver.New(driver.Options{
		Engine: e, Transport: tr, Render: r.render, Sink: r,
		Settle: driver.SettleOptions{First: 5 * time.Second, Quiet: 2 * time.Second, Max: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := d.Sleep(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("settle ran %v; Max did not cap it", elapsed)
	}
	if len(e.Written) == 0 {
		t.Fatal("settle absorbed nothing from the stream")
	}
}

func TestDeterminism(t *testing.T) {
	script := func() []frameRec {
		g := newRig(t, true)
		ctx := context.Background()
		if err := g.d.Type(ctx, "make", 30*time.Millisecond); err != nil {
			t.Fatal(err)
		}
		if err := g.d.Sleep(ctx, time.Second); err != nil {
			t.Fatal(err)
		}
		if err := g.d.Finish(); err != nil {
			t.Fatal(err)
		}
		return g.r.frames
	}
	a, b := script(), script()
	if len(a) != len(b) {
		t.Fatalf("runs differ in length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("frame %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

var _ driver.Transport = (*ptyx.Proc)(nil)
