//go:build ghosttyvt

package ghostty_test

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/ghostty"
)

// TestPipelinePhoto exercises the whole pipeline end to end: a real shell
// on a real pty prints styled text AND transmits a kitty-graphics image;
// the engine parses everything; the rasterizer produces the frame. Set
// FOLEY_PHOTO_OUT to keep the PNG.
func TestPipelinePhoto(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "..", "fontpack", "fonts"))
	if err != nil {
		t.Skipf("fontpack: %v", err)
	}
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	cellW, cellH := r.LogicalCellSize()
	geo := vtengine.Geometry{Cols: 64, Rows: 14, CellW: cellW, CellH: cellH}

	// A small gradient PNG that the DEMO PROCESS transmits through the pty.
	imgPath := filepath.Join(t.TempDir(), "demo.png")
	writeGradientPNG(t, imgPath, 64, 64)

	script := `
printf '\e[1;38;2;137;180;250mfoley\e[0m: primera foto real del pipeline\r\n'
printf 'proceso real %s pty %s libghostty-vt %s raster pure-Go\r\n' '->' '->' '->'
printf '\e[38;2;166;227;161mligaduras\e[0m: -> => != fi  \e[3;38;2;243;139;168mitalic\e[0m \e[1mbold\e[0m \e[4munder\e[0m\r\n'
printf 'kitty graphics desde el proceso:\r\n'
printf '\033_Ga=T,f=100,i=1,c=16,r=8,q=2;%s\033\\' "$(base64 < IMGPATH | tr -d '\n')"
`
	script = strings.ReplaceAll(script, "IMGPATH", imgPath)

	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", script},
		Size:    ptyx.Winsize{Cols: geo.Cols, Rows: geo.Rows, XPix: geo.Cols * geo.CellW, YPix: geo.Rows * geo.CellH},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	e, err := ghostty.New(vtengine.Options{
		Geometry:          geo,
		KittyStorageLimit: 8 << 20,
		Responses:         p,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

	// Pump until the process exits (the channel closes) or timeout.
	deadline := time.After(5 * time.Second)
pump:
	for {
		select {
		case c, ok := <-p.Chunks():
			if !ok {
				break pump
			}
			if _, err := e.Write(c.Data); err != nil {
				t.Fatal(err)
			}
		case <-deadline:
			t.Fatal("timeout pumping demo output")
		}
	}

	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatal(err)
	}
	if len(f.Graphics.Placements) == 0 {
		t.Fatalf("the demo process transmitted an image but no placement arrived; screen:\n%s", f.Text())
	}
	out, err := r.Render(&f, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	dest := os.Getenv("FOLEY_PHOTO_OUT")
	if dest == "" {
		dest = filepath.Join(t.TempDir(), "photo.png")
	}
	fd, err := os.Create(dest) //nolint:gosec // test artifact path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fd.Close() }()
	if err := png.Encode(fd, out); err != nil {
		t.Fatal(err)
	}
	t.Logf("photo: %s (%dx%d)", dest, out.Bounds().Dx(), out.Bounds().Dy())
}

func writeGradientPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(64 + x*2), G: uint8(96 + y*2), B: 0xf4, A: 0xff, //nolint:gosec // bounded by w,h=64
			})
		}
	}
	fd, err := os.Create(path) //nolint:gosec // temp path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fd.Close() }()
	if err := png.Encode(fd, img); err != nil {
		t.Fatal(err)
	}
}
