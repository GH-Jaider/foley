//go:build ghosttyvt

package tape_test

import (
	"bytes"
	"context"
	"image/gif"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/tape"
)

// TestMigratedTapeEndToEnd is the ADR-008 promise executed: a tape
// written in plain VHS — settings, typing, Ctrl chords, prompt Wait,
// screenshot, multiple outputs — runs against a REAL bash on the real
// engine and produces the artifacts. The staged WindowBar setting must
// warn loudly, never silently.
func TestMigratedTapeEndToEnd(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	fonts, err := filepath.Abs(filepath.Join("..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}
	if _, err := execx.LookPath("bash"); err != nil {
		testassets.Require(t, err, "bash on PATH")
	}

	// VHS tapes use paths relative to the tape's directory; run there,
	// exactly like the CLI will.
	dir := t.TempDir()
	t.Chdir(dir)
	gifPath := filepath.Join(dir, "demo.gif")
	txtPath := filepath.Join(dir, "final.txt")
	shotPath := filepath.Join(dir, "captura.png")

	src := `Output demo.gif
Output final.txt
Require bash
Set Shell bash
Set FontSize 22
Set Width 640
Set Height 240
Set Padding 20
Set TypingSpeed 40ms
Set WindowBar Colorful
Set WaitTimeout 20s
Sleep 300ms
Type "echo hola foley"
Enter
Wait
Sleep 600ms
Screenshot captura.png
`
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := tape.Run(ctx, tp, tape.RunOptions{FontsDir: fonts})
	if err != nil {
		t.Fatalf("run: %v (warnings: %v)", err, rep.Warnings)
	}

	if len(rep.Outputs) != 2 {
		t.Fatalf("outputs = %v", rep.Outputs)
	}
	// Chrome is RENDERED since the dress round — a WindowBar warning
	// would mean it silently regressed to staged.
	for _, w := range rep.Warnings {
		if strings.Contains(w, "WindowBar") {
			t.Fatalf("WindowBar warned as staged, but chrome is implemented: %v", rep.Warnings)
		}
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
	// prompt + 15 keystrokes + output/prompt + trailing hold.
	if len(g.Image) < 16 {
		t.Fatalf("gif has %d frames, want a typing animation", len(g.Image))
	}

	text, err := os.ReadFile(txtPath) //nolint:gosec // path built from TempDir
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "hola foley") {
		t.Fatalf("final text lacks the command output:\n%s", text)
	}
	if !strings.Contains(string(text), ">") {
		t.Fatalf("final text lacks the VHS prompt:\n%s", text)
	}
	if _, err := os.Stat(shotPath); err != nil {
		t.Fatalf("screenshot missing: %v", err)
	}
}

// TestCustomPromptEndToEnd proves ADR-017 against a real bash: the
// tape's Env PS1 WINS over the shell table (the screen shows ❯, not
// the pinned >), and a bare Wait succeeds because it now expects the
// DERIVED pattern — a timeout here means either the env layering or
// the derivation regressed.
func TestCustomPromptEndToEnd(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	fonts, err := filepath.Abs(filepath.Join("..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}
	if _, err := execx.LookPath("bash"); err != nil {
		testassets.Require(t, err, "bash on PATH")
	}

	dir := t.TempDir()
	t.Chdir(dir)

	src := `Output final.txt
Require bash
Set Shell bash
Set Width 640
Set Height 200
Set WaitTimeout 20s
Env PS1 "❯ "
Sleep 300ms
Type "echo prompt propio"
Enter
Wait
Sleep 400ms
`
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := tape.Run(ctx, tp, tape.RunOptions{FontsDir: fonts})
	if err != nil {
		t.Fatalf("run: %v (warnings: %v)", err, rep.Warnings)
	}
	derived := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "custom prompt") {
			derived = true
		}
	}
	if !derived {
		t.Fatalf("the derivation notice is missing: %v", rep.Warnings)
	}
	text, err := os.ReadFile(filepath.Join(dir, "final.txt")) //nolint:gosec // TempDir path
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "❯") {
		t.Fatalf("the custom prompt never reached the screen (Env PS1 lost the env merge?):\n%s", text)
	}
	if !strings.Contains(string(text), "prompt propio") {
		t.Fatalf("final text lacks the command output:\n%s", text)
	}
}

// TestAllCuesTogetherEndToEnd pins the composition: dress + keys +
// highlight in ONE tape. The overlay mux, the reel's band arithmetic
// and the highlight paint must coexist — this is the tape a real user
// writes, not three lab tapes.
func TestAllCuesTogetherEndToEnd(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	fonts, err := filepath.Abs(filepath.Join("..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}
	if _, err := execx.LookPath("bash"); err != nil {
		testassets.Require(t, err, "bash on PATH")
	}
	dir := t.TempDir()
	t.Chdir(dir)

	src := `Output demo.gif
Require bash
Set Shell bash
Set Width 640
Set Height 220
# foley: dress noir
# foley: keys small
Type "echo all cues"
Enter
Sleep 500ms
# foley: highlight /^all cues/
Sleep 800ms
# foley: highlight off
Sleep 300ms
`
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := tape.Run(ctx, tp, tape.RunOptions{FontsDir: fonts})
	if err != nil {
		t.Fatalf("run: %v (warnings: %v)", err, rep.Warnings)
	}
	f, err := os.Open(filepath.Join(dir, "demo.gif")) //nolint:gosec // TempDir path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	// Typing frames + reel fades + the highlight's two snap frames.
	if len(g.Image) < 12 {
		t.Fatalf("gif has %d frames, want the full composition", len(g.Image))
	}
}

// TestProgressPulseEndToEnd records a real tape collecting the
// progress pulse: phases in order (recording → developing per output),
// a monotonic clock, the declared total as an exact promise, and a
// growing frame count.
func TestProgressPulseEndToEnd(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	fonts, err := filepath.Abs(filepath.Join("..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}
	if _, err := execx.LookPath("bash"); err != nil {
		testassets.Require(t, err, "bash on PATH")
	}
	dir := t.TempDir()
	t.Chdir(dir)

	src := `Output demo.gif
Require bash
Set Shell bash
Set Width 480
Set Height 200
Set TypingSpeed 20ms
Type "echo pulso"
Enter
Sleep 400ms
`
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	var events []tape.ProgressEvent
	rep, err := tape.Run(ctx, tp, tape.RunOptions{
		FontsDir: fonts,
		Progress: func(ev tape.ProgressEvent) { events = append(events, ev) },
	})
	if err != nil {
		t.Fatalf("run: %v (warnings: %v)", err, rep.Warnings)
	}
	// One start pulse + one per command (3) + one developing per output.
	if len(events) != 5 {
		t.Fatalf("events = %d (%+v), want 5", len(events), events)
	}
	// "echo pulso" = 10 runes x 20ms + Enter 20ms + Sleep 400ms = 620ms.
	want := 620 * time.Millisecond
	prev := time.Duration(-1)
	for i, ev := range events {
		if ev.Total != want {
			t.Fatalf("event %d total = %v, want %v", i, ev.Total, want)
		}
		if ev.Now < prev {
			t.Fatalf("clock went backwards at event %d: %v after %v", i, ev.Now, prev)
		}
		prev = ev.Now
		if wantPhase := tape.ProgressRecording; i == len(events)-1 {
			wantPhase = tape.ProgressDeveloping
			if ev.Output != "demo.gif" {
				t.Fatalf("developing output = %q", ev.Output)
			}
			if ev.Frames == 0 {
				t.Fatal("developing pulse reports zero frames")
			}
			if ev.Phase != wantPhase {
				t.Fatalf("event %d phase = %v, want %v", i, ev.Phase, wantPhase)
			}
		} else if ev.Phase != tape.ProgressRecording {
			t.Fatalf("event %d phase = %v, want recording", i, ev.Phase)
		}
	}
	if final := events[len(events)-1]; final.Now < want {
		t.Fatalf("final clock %v never reached the declared total %v", final.Now, want)
	}
}

// TestZoomEndToEnd records a real tape through the camera (ADR-019):
// the canvas keeps its DECLARED size while the master renders at 2×,
// the transitions add their quantized frames — and, the constitutional
// claim, two identical runs produce byte-identical output.
func TestZoomEndToEnd(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	fonts, err := filepath.Abs(filepath.Join("..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}
	if _, err := execx.LookPath("bash"); err != nil {
		testassets.Require(t, err, "bash on PATH")
	}
	dir := t.TempDir()
	t.Chdir(dir)

	src := `Output demo.gif
Require bash
Set Shell bash
Set Width 640
Set Height 220
Type "echo zoom aqui"
Enter
Sleep 400ms
# foley: zoom 0,0 30x6 400ms
Sleep 600ms
# foley: zoom off 400ms
Sleep 500ms
`
	run := func() []byte {
		tp, err := tape.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		rep, err := tape.Run(ctx, tp, tape.RunOptions{FontsDir: fonts})
		if err != nil {
			t.Fatalf("run: %v (warnings: %v)", err, rep.Warnings)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "demo.gif")) //nolint:gosec // TempDir path
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	first := run()
	g, err := gif.DecodeAll(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	// The house output is retina: declared × Scale 2. The zoom master
	// (another 2× on top) must stay internal — a leak would double this.
	if w, h := g.Config.Width, g.Config.Height; w != 1280 || h != 440 {
		t.Fatalf("canvas = %dx%d, want 1280x440 (declared 640x220 at the standard 2×) — the master must stay internal", w, h)
	}
	// Two eased moves at 400ms are ~12 quantized cuts each on top of
	// the typing frames.
	if len(g.Image) < 12 {
		t.Fatalf("gif has %d frames, want the zoom transitions in it", len(g.Image))
	}
	second := run()
	if !bytes.Equal(first, second) {
		t.Fatal("two identical zoom runs differ — the camera broke byte-determinism")
	}
}

// TestHighlightRealtimeEndToEnd smokes the wall clock (ADR-018): the
// track mutates from the recording goroutine while the loop renders —
// the mutex and the tick gating must hold on a REAL recording.
func TestHighlightRealtimeEndToEnd(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	fonts, err := filepath.Abs(filepath.Join("..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}
	if _, err := execx.LookPath("bash"); err != nil {
		testassets.Require(t, err, "bash on PATH")
	}
	dir := t.TempDir()
	t.Chdir(dir)

	src := `Output demo.gif
Require bash
Set Shell bash
Set Width 640
Set Height 200
Type@0ms "echo vivo"
Enter
Sleep 300ms
# foley: highlight /vivo/
Sleep 400ms
# foley: highlight off
Sleep 200ms
`
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := tape.Run(ctx, tp, tape.RunOptions{FontsDir: fonts, Mode: foley.Realtime})
	if err != nil {
		t.Fatalf("realtime run: %v (warnings: %v)", err, rep.Warnings)
	}
	if _, err := os.Stat(filepath.Join(dir, "demo.gif")); err != nil {
		t.Fatalf("gif missing: %v", err)
	}
}

// TestZoomRealtimeEndToEnd smokes the camera on the wall clock
// (ADR-019): the track mutates from the recording goroutine while the
// loop composits at 2×; the tick gating must emit the transition frames
// on a REAL recording.
func TestZoomRealtimeEndToEnd(t *testing.T) {
	ctx := context.Background()
	_, err := execx.Find(ctx, execx.FFmpeg)
	testassets.Require(t, err, "install ffmpeg (the CI workflow installs it)")
	fonts, err := filepath.Abs(filepath.Join("..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); err != nil {
		testassets.Require(t, err, "make fonts")
	}
	if _, err := execx.LookPath("bash"); err != nil {
		testassets.Require(t, err, "bash on PATH")
	}
	dir := t.TempDir()
	t.Chdir(dir)

	src := `Output demo.gif
Require bash
Set Shell bash
Set Width 640
Set Height 220
Type@0ms "echo vivo"
Enter
Sleep 300ms
# foley: zoom 0,0 30x6 300ms
Sleep 500ms
# foley: zoom off 300ms
Sleep 400ms
`
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := tape.Run(ctx, tp, tape.RunOptions{FontsDir: fonts, Mode: foley.Realtime})
	if err != nil {
		t.Fatalf("realtime run: %v (warnings: %v)", err, rep.Warnings)
	}
	f, err := os.Open(filepath.Join(dir, "demo.gif")) //nolint:gosec // TempDir path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := g.Config.Width, g.Config.Height; w != 1280 || h != 440 {
		t.Fatalf("canvas = %dx%d, want 1280x440 — the master must stay internal in realtime too", w, h)
	}
	// The transitions must materialize as EXTRA frames beyond the
	// static screen even though nothing typed during them (the overlay
	// tick gating). Structural bound, not a count: under load (-race,
	// busy CI) the wall-clock loop legitimately coalesces ticks — the
	// exact quantization is pinned by the deterministic e2e instead.
	if len(g.Image) < 4 {
		t.Fatalf("gif has %d frames, want the zoom transitions captured", len(g.Image))
	}
}
