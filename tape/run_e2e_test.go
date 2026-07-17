//go:build ghosttyvt

package tape_test

import (
	"context"
	"image/gif"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
