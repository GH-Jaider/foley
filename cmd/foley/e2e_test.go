//go:build ghosttyvt

package main

import (
	"bytes"
	"context"
	"image/gif"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/internal/testassets"
)

// TestCLIEndToEnd is M8's definition of done: `foley demo.tape` produces
// a correct gif and mp4.
func TestCLIEndToEnd(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	fonts, err := filepath.Abs(filepath.Join("..", "..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}

	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, "demo.tape"), `Output demo.gif
Output demo.mp4
Set Width 640
Set Height 240
Set TypingSpeed 30ms
Set WaitTimeout 20s
Sleep 300ms
Type "echo listo"
Enter
Wait
Sleep 500ms
`)

	var out, errb bytes.Buffer
	if got := run([]string{"-fonts", fonts, "demo.tape"}, &out, &errb); got != 0 {
		t.Fatalf("exit = %d\nstderr: %s", got, errb.String())
	}
	if !strings.Contains(out.String(), "wrote demo.gif") || !strings.Contains(out.String(), "wrote demo.mp4") {
		t.Fatalf("stdout = %q", out.String())
	}

	f, err := os.Open(filepath.Join(dir, "demo.gif")) //nolint:gosec // path built from TempDir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) < 10 {
		t.Fatalf("gif has %d frames", len(g.Image))
	}
	info, err := os.Stat(filepath.Join(dir, "demo.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 500 {
		t.Fatalf("mp4 suspiciously small: %d bytes", info.Size())
	}
}

// TestKittyGraphicsExample runs examples/kitty-graphics EXACTLY as
// shipped (copied to a tempdir, CLI-driven): the demo transmits a PNG
// through the kitty graphics protocol and its four quadrant colors must
// survive to actual GIF pixels — the recording VHS cannot make.
func TestKittyGraphicsExample(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	fonts, err := filepath.Abs(filepath.Join("..", "..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}
	src, err := filepath.Abs(filepath.Join("..", "..", "examples", "kitty-graphics"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	for _, name := range []string{"demo.tape", "show.sh"} {
		data, err := os.ReadFile(filepath.Join(src, name)) //nolint:gosec // repo example path
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, name), string(data))
	}
	t.Chdir(dir)

	var out, errb bytes.Buffer
	if got := run([]string{"-fonts", fonts, "demo.tape"}, &out, &errb); got != 0 {
		t.Fatalf("exit = %d\nstderr: %s", got, errb.String())
	}

	f, err := os.Open(filepath.Join(dir, "demo.gif")) //nolint:gosec // path built from TempDir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	// The transmitted image's quadrants: magenta, cyan, yellow, green.
	quads := []struct {
		name    string
		r, g, b int
	}{
		{"magenta", 0xf5, 0x00, 0xa5},
		{"cyan", 0x00, 0xd7, 0xff},
		{"yellow", 0xff, 0xe1, 0x00},
		{"green", 0x28, 0xe0, 0x6e},
	}
	found := make([]bool, len(quads))
	for _, frame := range g.Image {
		for i := range frame.Pix {
			r, gg, b, _ := frame.Palette[frame.Pix[i]].RGBA()
			r8, g8, b8 := int(r>>8), int(gg>>8), int(b>>8)
			for qi, q := range quads {
				if abs(r8-q.r) <= 8 && abs(g8-q.g) <= 8 && abs(b8-q.b) <= 8 {
					found[qi] = true
				}
			}
		}
	}
	for qi, q := range quads {
		if !found[qi] {
			t.Fatalf("kitty image color %s missing from the GIF (found=%v)", q.name, found)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
