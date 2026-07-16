// Command foley renders a VHS .tape script into gif/mp4/webm/txt/frames
// without a terminal window: the demo runs on a real pty against an
// embedded terminal engine and foley rasterizes every frame itself. It
// is a thin consumer of the public API (library first): flags in,
// tape.Run out.
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

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/tape"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main minus the process boundary, so tests can drive it.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("foley", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "deterministic",
		"recording clock: deterministic (virtual time, byte-identical output) or realtime")
	mok := fs.Bool("modify-other-keys", false,
		"encode modified keys (Ctrl+Enter, Shift+Tab...) with xterm's modern CSI-27 forms instead of degrading them like xterm.js/VHS")
	fonts := fs.String("fonts", "",
		"directory holding the pinned fonts (default: $FOLEY_FONTS, then ./fonts)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "usage: foley [flags] <file.tape>\n\n"+
			"Relative paths in the tape (Output, Screenshot, Source) resolve\n"+
			"against the current working directory, exactly like VHS.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}

	opts := tape.RunOptions{
		ModifyOtherKeys: *mok,
		FontsDir:        *fonts,
		Warn:            stderr,
	}
	switch *mode {
	case "deterministic":
		opts.Mode = foley.Deterministic
	case "realtime":
		opts.Mode = foley.Realtime
	default:
		_, _ = fmt.Fprintf(stderr, "foley: unknown mode %q (deterministic|realtime)\n", *mode)
		return 2
	}

	src, err := os.ReadFile(fs.Arg(0)) //nolint:gosec // the tape path is the CLI's whole purpose
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
		return 1
	}
	t, err := tape.Parse(string(src))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	// Ctrl-C cancels the context: actions fail fast and the Recorder's
	// cleanup (process, engine, staging) runs instead of leaking.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rep, err := tape.Run(ctx, t, opts)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
		return 1
	}
	for _, out := range rep.Outputs {
		_, _ = fmt.Fprintf(stdout, "foley: wrote %s\n", out)
	}
	return 0
}
