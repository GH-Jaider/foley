//go:build ghosttyvt

package tape_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// TestMigratedTapeEndToEnd is the compatibility promise executed: a tape
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

// TestCustomPromptEndToEnd proves the custom-prompt design against a real bash: the
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

// TestTerminalIdentityEndToEnd pins the terminal identity against a real recording
// with a POLLUTED host environment: inside the tape, TERM_PROGRAM says
// foley, the host terminal's kitty marker is gone, and the tape's own
// explicit Env still wins over the identity layer.
func TestTerminalIdentityEndToEnd(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("KITTY_WINDOW_ID", "7")
	ctx := context.Background()
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
Set Width 760
Set Height 220
Env DEMO_ID "propio"
Type@0ms "echo P=$TERM_PROGRAM T=$TERM K=[$KITTY_WINDOW_ID] D=$DEMO_ID"
Enter
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
	text, err := os.ReadFile(filepath.Join(dir, "final.txt")) //nolint:gosec // TempDir path
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"P=foley", "T=xterm-ghostty", "K=[]", "D=propio"} {
		if !strings.Contains(string(text), want) {
			t.Fatalf("final screen lacks %q — the identity layer leaked:\n%s", want, text)
		}
	}
}

// TestOutputScaleEndToEnd pins the weight knob: OutputScale 1 halves
// the canvas to LOGICAL size — with the camera in the same take, so
// the zoom compositor and the final halving compose.
func TestOutputScaleEndToEnd(t *testing.T) {
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
Type@0ms "echo liviano"
Enter
Sleep 300ms
# foley: zoom 0,0 30x6 200ms
Sleep 400ms
`
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := tape.Run(ctx, tp, tape.RunOptions{FontsDir: fonts, OutputScale: 1})
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
	if w, h := g.Config.Width, g.Config.Height; w != 640 || h != 220 {
		t.Fatalf("canvas = %dx%d, want the LOGICAL 640x220 at OutputScale 1", w, h)
	}
}

// TestCastEndToEnd records a tape straight to asciicast v2: the header
// carries the real grid, events parse with monotonic exact timestamps,
// the typed text is in the stream — and two identical runs produce a
// byte-identical cast (same-instant merging makes pty chunking noise
// vanish).
func TestCastEndToEnd(t *testing.T) {
	ctx := context.Background()
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

	src := `Output demo.cast
Require bash
Set Shell bash
Set Width 480
Set Height 200
Type@0ms "echo casteado"
Enter
Sleep 300ms
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
		raw, err := os.ReadFile(filepath.Join(dir, "demo.cast")) //nolint:gosec // TempDir path
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	first := run()
	lines := strings.Split(strings.TrimRight(string(first), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("cast has %d lines, want header + events:\n%s", len(lines), first)
	}
	var header struct {
		Version       int `json:"version"`
		Width, Height int
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("header %q: %v", lines[0], err)
	}
	if header.Version != 2 || header.Width <= 0 || header.Height <= 0 {
		t.Fatalf("header = %+v, want version 2 with the real grid", header)
	}
	prev := -1.0
	var all strings.Builder
	for _, ln := range lines[1:] {
		var ev []any
		if err := json.Unmarshal([]byte(ln), &ev); err != nil {
			t.Fatalf("event %q: %v", ln, err)
		}
		if len(ev) != 3 || ev[1] != "o" {
			t.Fatalf("event %q: want [time, \"o\", data]", ln)
		}
		at, ok := ev[0].(float64)
		if !ok || at < prev {
			t.Fatalf("event %q: non-monotonic time (prev %v)", ln, prev)
		}
		prev = at
		s, _ := ev[2].(string)
		all.WriteString(s)
	}
	if !strings.Contains(all.String(), "casteado") {
		t.Fatalf("typed text missing from the stream:\n%s", all.String())
	}
	second := run()
	if !bytes.Equal(first, second) {
		t.Fatal("two identical runs differ — the cast broke byte-determinism")
	}
}

// TestZoomEndToEnd records a real tape through the camera:
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

// TestHighlightRealtimeEndToEnd smokes the wall clock: the
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

// TestZoomRealtimeEndToEnd smokes the camera on the wall clock:
// the track mutates from the recording goroutine while the
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
Sleep 600ms
# foley: zoom 0,0 30x6 800ms
Sleep 1s
# foley: zoom off 800ms
Sleep 900ms
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
	// The take spans ~4s because the slowest runner observed live (CI
	// under -race, first release day) ticked at ~1.2 fps and starved a
	// 1.5s take down to 3 frames. If this bound EVER fires again, the
	// durable fix is not more margin: it is a driver seam that renders
	// camera-phase boundaries (cue arrival, transition end) regardless
	// of ticker starvation — ADR territory, deliberately not patched
	// into the realtime loop overnight.
	if len(g.Image) < 4 {
		t.Fatalf("gif has %d frames, want the zoom transitions captured", len(g.Image))
	}
}

// TestStudioEndToEnd proves the studio against a real bash: the take runs
// INSIDE the set (the working directory is the set's home, $USER is the
// set's identity), the tape's own Env still wins over the studio layer
// (HOSTNAME reads the tape's value, not the set's), and the set is
// struck by the time Run returns — nothing of the take on the host,
// nothing of the host's home on camera.
func TestStudioEndToEnd(t *testing.T) {
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

	// FontSize 14 at Width 1200 leaves ~140 columns: the set's absolute
	// path must fit on ONE line for the strike check below to read it.
	src := `Output final.txt
Require bash
Set Shell bash
Set Width 1200
Set Height 400
Set FontSize 14
Set WaitTimeout 20s
# foley: studio
Env HOSTNAME "backlot"
Sleep 300ms
Type "echo $USER@$HOSTNAME"
Enter
Wait
Type "pwd"
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
	text, err := os.ReadFile(filepath.Join(dir, "final.txt")) //nolint:gosec // TempDir path
	if err != nil {
		t.Fatal(err)
	}
	// USER comes from the studio layer; HOSTNAME from the tape's Env —
	// one line proves both the layer and its place in the merge order.
	if !strings.Contains(string(text), "foley@backlot") {
		t.Fatalf("screen lacks foley@backlot (studio layer or merge order regressed):\n%s", text)
	}
	var setPath string
	for _, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/") && strings.Contains(line, "foley-set-") {
			setPath = line
			break
		}
	}
	if setPath == "" {
		t.Fatalf("pwd printed no set path:\n%s", text)
	}
	if !strings.HasSuffix(setPath, "/home") {
		t.Fatalf("working directory %q is not the set's home", setPath)
	}
	root := strings.TrimSuffix(setPath, "/home")
	if _, err := os.Stat(root); !os.IsNotExist(err) { //nolint:gosec // the set's own path read back from the screen, asserting absence
		t.Fatalf("the set was not struck: stat %v", err)
	}
	// len > 1 guards the degenerate HOME=/ (a bare root would match
	// every absolute path on screen, including the set's own).
	if home, herr := os.UserHomeDir(); herr == nil && len(home) > 1 && strings.Contains(string(text), home) {
		t.Fatalf("the host home %q is on camera:\n%s", home, text)
	}
}
