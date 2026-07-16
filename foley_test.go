package foley

import (
	"context"
	"errors"
	"image/gif"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
)

func TestNewRequiresCommand(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("New without Command must error")
	}
}

// fontsMissing turns a non-font-related error into nil so
// testassets.Require only gates on the actual missing-fonts case.
func fontsMissing(err error) error {
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// TestRecorderPublicFlow drives the whole public surface over the fake
// engine and a real pty: the entire Recorder wiring minus the cgo engine
// (which the tagged e2e covers).
func TestRecorderPublicFlow(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	if _, err := os.Stat(filepath.Join("internal", "fontpack", "fonts", "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}

	opts := Options{
		Command: []string{"/bin/sh", "-c", `read line; printf 'got: %s' "$line"`},
		Cols:    40, Rows: 4,
		FontsDir: filepath.Join("internal", "fontpack", "fonts"),
	}
	applyDefaults(&opts)
	rec, err := assembleRecorder(opts, fake.New(vtengine.Options{
		Geometry: vtengine.Geometry{Cols: 40, Rows: 4},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rec.Close() }()

	if err := rec.Type(ctx, "hola", 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	// Nested path: parent directories must be created (VHS does).
	shot := filepath.Join(t.TempDir(), "capturas", "demo", "captura demo.png")
	if err := rec.Screenshot(shot); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shot); err != nil {
		t.Fatalf("screenshot not delivered: %v", err)
	}
	if err := rec.WaitText(ctx, regexp.MustCompile(`hola`), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := rec.Sleep(ctx, time.Second); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "demo.gif")
	if err := rec.Output(ctx, out); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out) //nolint:gosec // path built from TempDir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) < 2 {
		t.Fatalf("gif has %d frames, want typing animation", len(g.Image))
	}

	// The timeline is sealed after Output; a second Output re-encodes.
	if err := rec.Type(ctx, "x", 0); !errors.Is(err, ErrFinished) {
		t.Fatalf("action after Output = %v, want ErrFinished", err)
	}
	if err := rec.Output(ctx, filepath.Join(t.TempDir(), "demo.mp4")); err != nil {
		t.Fatal(err)
	}
	if err := rec.Output(ctx, filepath.Join(t.TempDir(), "demo.avi")); !errors.Is(err, ErrUnsupportedOutput) {
		t.Fatalf("avi = %v, want ErrUnsupportedOutput", err)
	}

	framesDir := rec.framesDir
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil { // idempotent
		t.Fatal(err)
	}
	if _, err := os.Stat(framesDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging dir must be removed on Close: %v", err)
	}
}

// TestRecorderRealtimeFlow smoke-tests the OTHER clock through the
// public wiring: the sampling loop starts inside the constructor with
// the pty already live. Content-only assertions, like every realtime
// test.
func TestRecorderRealtimeFlow(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	if _, err := os.Stat(filepath.Join("internal", "fontpack", "fonts", "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}

	opts := Options{
		Command:  []string{"/bin/sh", "-c", "printf listo; read hold"},
		Cols:     40,
		Rows:     4,
		Mode:     Realtime,
		FPS:      100,
		FontsDir: filepath.Join("internal", "fontpack", "fonts"),
	}
	applyDefaults(&opts)
	rec, err := assembleRecorder(opts, fake.New(vtengine.Options{
		Geometry: vtengine.Geometry{Cols: 40, Rows: 4},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rec.Close() }()

	if err := rec.WaitText(ctx, regexp.MustCompile(`listo`), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "rt.gif")
	if err := rec.Output(ctx, out); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out) //nolint:gosec // path built from TempDir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) == 0 {
		t.Fatal("realtime recording emitted no frames")
	}
	// .txt after Output must work in realtime too: the loop is gone, but
	// finish() captured the closing screen first (found the hard way).
	txt := filepath.Join(t.TempDir(), "rt.txt")
	if err := rec.Output(ctx, txt); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(txt) //nolint:gosec // path built from TempDir
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`listo`).Match(data) {
		t.Fatalf("realtime .txt lacks the final screen: %q", data)
	}
}

func TestUnknownModeFails(t *testing.T) {
	opts := Options{
		Command:  []string{"/bin/sh", "-c", "true"},
		Mode:     Mode(99),
		FontsDir: filepath.Join("internal", "fontpack", "fonts"),
	}
	applyDefaults(&opts)
	_, err := assembleRecorder(opts, fake.New(vtengine.Options{
		Geometry: vtengine.Geometry{Cols: 80, Rows: 24},
	}))
	testassets.Require(t, fontsMissing(err), "make fonts")
	if err == nil || !regexp.MustCompile(`unknown mode`).MatchString(err.Error()) {
		t.Fatalf("err = %v, want unknown mode", err)
	}
}
