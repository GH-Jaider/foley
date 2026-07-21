package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/internal/preview"
	"github.com/GH-Jaider/foley/tape"
)

// playHandshakeTimeout bounds the kitty-graphics handshake: a terminal
// that speaks the protocol answers in single-digit milliseconds even
// over ssh; the sentinel DA1 reply usually closes it long before this.
const playHandshakeTimeout = 400 * time.Millisecond

// runPlay records the tape and replays it in the user's own terminal
// via kitty graphics. Terminals that don't speak the
// protocol — decided by handshake, not env sniffing — get the honest
// fallback: the tape's first output opened with the system viewer.
func runPlay(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("foley play", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "deterministic",
		"recording clock: deterministic (virtual time) or realtime")
	mok := fs.Bool("modify-other-keys", false,
		"encode modified keys with xterm's modern CSI-27 forms instead of degrading them")
	fonts := fs.String("fonts", "",
		"directory holding the pinned fonts (default: $FOLEY_FONTS, then ./fonts)")
	dress := fs.String("dress", "",
		"replace the tape's dress layer (same forms as the record flag)")
	keys := fs.String("keys", "",
		"replace the tape's keys layer: off, or comma-separated keys tokens (on, small|medium|large, notation=keycap|icons, accent=<ansi|#hex|off>, plain)")
	themeFlag := fs.String("theme", "",
		"replace the recording's theme (a curated name or an inline {json})")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "usage: foley play [flags] <file.tape | ->\n\n"+
			"Your terminal is the screen: records the tape and replays it right\n"+
			"here via kitty graphics — no files opened, no windows. The tape's\n"+
			"declared outputs are still written. A terminal without kitty\n"+
			"graphics gets the first output screened in the system viewer.\n\n")
		fs.PrintDefaults()
	}
	files, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if len(files) != 1 {
		fs.Usage()
		return 2
	}
	m, ok := parseMode(*mode)
	if !ok {
		_, _ = fmt.Fprintf(stderr, "foley: unknown mode %q (deterministic|realtime)\n", *mode)
		return 2
	}
	var dressRef tape.DressRef
	if *dress != "" {
		var err error
		dressRef, err = tape.ParseDressRef(resolveDressArg(*dress))
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: -dress: %v\n", err)
			return 2
		}
	}
	keysOverride, err := parseKeysOverride(*keys)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: -keys %q: %v\n", *keys, err)
		return 2
	}
	var themeRef tape.ThemeRef
	if *themeFlag != "" {
		var err error
		themeRef, err = tape.ParseThemeRef(*themeFlag)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: -theme: %v\n", err)
			return 2
		}
	}

	// The stage check comes FIRST — before minutes of recording: play
	// needs a controlling terminal to draw on (or to know it must fall
	// back). A pipe has neither.
	tty, ttyErr := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if ttyErr != nil {
		_, _ = fmt.Fprintln(stderr, "foley: play needs a terminal (no controlling tty) — record with `foley <tape>` instead")
		return 1
	}
	defer func() { _ = tty.Close() }()

	src, err := readTape(files[0], stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
		return 1
	}
	t, err := tape.Parse(string(src))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	progress := newProgressRenderer(stderr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rep, err := tape.Run(ctx, t, tape.RunOptions{
		Mode:            m,
		ModifyOtherKeys: *mok,
		FontsDir:        *fonts,
		Dress:           dressRef,
		Keys:            keysOverride,
		Theme:           themeRef,
		Warn:            progress.warnWriter(),
		Progress:        progress.pulse,
		KeepFrames:      true,
	})
	progress.done()
	if rep.FramesDir != "" {
		defer func() { _ = os.RemoveAll(rep.FramesDir) }() //nolint:gosec // the recorder's OWN staging dir (MkdirTemp), reported back by Run — not user input
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
		return 1
	}
	for _, out := range rep.Outputs {
		_, _ = fmt.Fprintf(stdout, "foley: wrote %s\n", out)
	}

	if preview.Supported(tty, playHandshakeTimeout) {
		if err := preview.Play(ctx, tty, rep.FramesDir); err != nil && !errors.Is(err, context.Canceled) {
			_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
			return 1
		}
		return 0
	}
	if len(rep.Outputs) == 0 {
		_, _ = fmt.Fprintln(stderr, "foley: this terminal doesn't speak kitty graphics and the tape declared no Output to screen")
		return 1
	}
	_, _ = fmt.Fprintf(stderr, "foley: this terminal doesn't speak kitty graphics — screening %s in the system viewer instead\n", rep.Outputs[0])
	if err := execx.OpenFile(ctx, rep.Outputs[0]); err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: %v (the recording is at %s)\n", err, rep.Outputs[0])
		return 1
	}
	return 0
}
