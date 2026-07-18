package encode_test

import (
	"context"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/encode"
	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/internal/testassets"
)

// TestAssembleGIFAndMP4 runs real ffmpeg over a tiny recording with ODD
// dimensions (the pad filter must fix them for yuv420p) and verifies the
// GIF by decoding it with the stdlib: exact frame count, positive delays.
func TestAssembleGIFAndMP4(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")

	dir := t.TempDir()
	s, err := encode.NewPNGSink(encode.PNGSinkOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	colors := []color.RGBA{
		{R: 0xf3, G: 0x8b, B: 0xa8, A: 0xff},
		{R: 0xa6, G: 0xe3, B: 0xa1, A: 0xff},
		{R: 0x89, G: 0xb4, B: 0xfa, A: 0xff},
	}
	durations := []time.Duration{300 * time.Millisecond, 60 * time.Millisecond, 1300 * time.Millisecond}
	for i, c := range colors {
		if err := s.Add(solid(101, 51, c), durations[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	gifPath := filepath.Join(dir, "out.gif")
	if err := encode.GIF(ctx, dir, gifPath, 0); err != nil {
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
	if len(g.Image) != len(colors) {
		t.Fatalf("gif has %d frames, want %d", len(g.Image), len(colors))
	}
	// EXACT delays in centiseconds — the assertion that catches both
	// timebase quantization (60ms must be 6cs, not 8/4) and the last
	// frame losing its duration to the -t cut.
	wantDelay := []int{30, 6, 130}
	for i, d := range g.Delay {
		if d != wantDelay[i] {
			t.Fatalf("gif delays = %v cs, want %v", g.Delay, wantDelay)
		}
	}

	mp4Path := filepath.Join(dir, "out.mp4")
	if err := encode.MP4(ctx, dir, mp4Path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(mp4Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 500 {
		t.Fatalf("mp4 suspiciously small: %d bytes", info.Size())
	}
}
