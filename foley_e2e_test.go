//go:build ghosttyvt

package foley_test

import (
	"context"
	"errors"
	"image/gif"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/key"
)

// TestPublicAPIRecording is the M8 definition-of-done in miniature: the
// exported surface alone — no internal imports beyond gates — records a
// real interactive session into a GIF with exact timing.
func TestPublicAPIRecording(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	if _, err := os.Stat(filepath.Join("internal", "fontpack", "fonts", "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}

	rec, err := foley.New(foley.Options{
		Command:  []string{"/bin/sh", "-c", `read line; printf 'ok: %s' "$line"`},
		Cols:     40,
		Rows:     4,
		FontsDir: filepath.Join("internal", "fontpack", "fonts"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rec.Close() }()

	if err := rec.Type(ctx, "api", 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := rec.Press(ctx, key.Key{Name: key.NameEnter}, 0); err != nil {
		t.Fatal(err)
	}
	if err := rec.WaitText(ctx, regexp.MustCompile(`ok: api`), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := rec.Sleep(ctx, 800*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got, want := rec.Now(), 950*time.Millisecond; got != want {
		t.Fatalf("virtual timeline = %v, want %v", got, want)
	}

	out := filepath.Join(t.TempDir(), "api.gif")
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
	// 3 keystrokes at exactly 5cs each; the tail (Enter + reply + hold)
	// sums to 80cs however the settle splits it.
	delays := g.Delay
	if len(delays) < 4 {
		t.Fatalf("gif has %d frames, want at least 4 (%v)", len(delays), delays)
	}
	for i := 0; i < 3; i++ {
		if delays[i] != 5 {
			t.Fatalf("keystroke %d delay = %dcs, want 5 (all: %v)", i, delays[i], delays)
		}
	}
	tail := 0
	for _, d := range delays[3:] {
		tail += d
	}
	if tail != 80 {
		t.Fatalf("tail sums %dcs, want 80 (all: %v)", tail, delays)
	}

	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rec.Sleep(ctx, time.Second); !errors.Is(err, foley.ErrFinished) {
		t.Fatalf("action after Close = %v, want ErrFinished", err)
	}
}
