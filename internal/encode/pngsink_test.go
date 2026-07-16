package encode_test

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/encode"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestPNGSinkManifestExact(t *testing.T) {
	dir := t.TempDir()
	s, err := encode.NewPNGSink(encode.PNGSinkOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	red := solid(4, 4, color.RGBA{R: 255, A: 255})
	if err := s.Add(red, 500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(red, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(red, 0); err != nil { // the legal Finish zero: clamped
		t.Fatal(err)
	}
	if err := s.Still("2024/informe final", red); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil { // idempotent
		t.Fatal(err)
	}

	want := `ffconcat version 1.0
file 'frame-00000.png'
option framerate 1000
duration 0.500000
file 'frame-00001.png'
option framerate 1000
duration 1.000000
file 'frame-00002.png'
option framerate 1000
duration 0.020000
file 'frame-00002.png'
option framerate 1000
`
	got, err := os.ReadFile(filepath.Join(dir, "frames.ffconcat")) //nolint:gosec // fixed name inside TempDir
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("manifest:\n%s\nwant:\n%s", got, want)
	}
	for _, name := range []string{"frame-00000.png", "frame-00001.png", "frame-00002.png", "still-2024_informe_final.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if s.Frames() != 3 {
		t.Fatalf("Frames() = %d", s.Frames())
	}
}

func TestPNGSinkConfigurableZeroDuration(t *testing.T) {
	dir := t.TempDir()
	s, err := encode.NewPNGSink(encode.PNGSinkOptions{Dir: dir, ZeroDuration: 750 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add(solid(2, 2, color.RGBA{A: 255}), 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "frames.ffconcat")) //nolint:gosec // fixed name inside TempDir
	if err != nil {
		t.Fatal(err)
	}
	if want := "duration 0.750000\n"; !strings.Contains(string(got), want) {
		t.Fatalf("manifest lacks configured zero-duration %q:\n%s", want, got)
	}
}

func TestPNGSinkEmptyRecordingErrors(t *testing.T) {
	s, err := encode.NewPNGSink(encode.PNGSinkOptions{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err == nil {
		t.Fatal("closing an empty recording must error")
	}
}

func TestPNGSinkRejectsUseAfterClose(t *testing.T) {
	s, err := encode.NewPNGSink(encode.PNGSinkOptions{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add(solid(1, 1, color.RGBA{A: 255}), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(solid(1, 1, color.RGBA{A: 255}), time.Second); err == nil {
		t.Fatal("Add after Close must error")
	}
}
