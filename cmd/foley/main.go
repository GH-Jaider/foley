// Command foley renders a VHS .tape script into gif/mp4/webm/txt/frames
// without a terminal window: the demo runs on a real pty against an
// embedded terminal engine and foley rasterizes every frame itself. It
// is a thin consumer of the public API (library first): flags in,
// tape.Run out. Besides recording, `foley validate` lints tapes without
// running them and `foley themes` lists the vendored theme catalog.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/tape"
)

// version is stamped by release builds (-ldflags "-X main.version=…");
// a plain `go install` answers from module build info, then "dev".
//
//nolint:gochecknoglobals // ldflags injection requires a package variable
var version string

func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// outputsFlag collects repeated -o values.
type outputsFlag []string

func (o *outputsFlag) String() string { return strings.Join(*o, ", ") }
func (o *outputsFlag) Set(v string) error {
	*o = append(*o, v)
	return nil
}

func parseMode(s string) (foley.Mode, bool) {
	switch s {
	case "deterministic":
		return foley.Deterministic, true
	case "realtime":
		return foley.Realtime, true
	}
	return foley.Deterministic, false
}

// readTape resolves a tape argument: a file path, or stdin for "-".
func readTape(arg string, stdin io.Reader) ([]byte, error) {
	if arg == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("stdin: %w", err)
		}
		return b, nil
	}
	return os.ReadFile(arg) //nolint:gosec // tape paths are the CLI's whole purpose
}

// run is main minus the process boundary, so tests can drive it.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "validate":
			return runValidate(args[1:], stdin, stderr)
		case "themes":
			return runThemes(args[1:], stdout, stderr)
		}
	}

	fs := flag.NewFlagSet("foley", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "deterministic",
		"recording clock: deterministic (virtual time, byte-identical output) or realtime")
	mok := fs.Bool("modify-other-keys", false,
		"encode modified keys (Ctrl+Enter, Shift+Tab...) with xterm's modern CSI-27 forms instead of degrading them like xterm.js/VHS")
	fonts := fs.String("fonts", "",
		"directory holding the pinned fonts (default: $FOLEY_FONTS, then ./fonts)")
	showVersion := fs.Bool("version", false, "print the foley version and exit")
	var outs outputsFlag
	fs.Var(&outs, "o",
		"write the recording to this path (repeatable; format by extension, replaces the tape's Output declarations)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "usage: foley [flags] <file.tape | ->\n"+
			"       foley validate [flags] <file.tape ... | ->\n"+
			"       foley themes\n\n"+
			"\"-\" reads the tape from stdin. Relative paths in the tape (Output,\n"+
			"Screenshot, Source) resolve against the current working directory,\n"+
			"exactly like VHS.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		_, _ = fmt.Fprintln(stdout, "foley "+versionString())
		return 0
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	m, ok := parseMode(*mode)
	if !ok {
		_, _ = fmt.Fprintf(stderr, "foley: unknown mode %q (deterministic|realtime)\n", *mode)
		return 2
	}

	opts := tape.RunOptions{
		Mode:            m,
		ModifyOtherKeys: *mok,
		FontsDir:        *fonts,
		Warn:            stderr,
	}
	src, err := readTape(fs.Arg(0), stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
		return 1
	}
	t, err := tape.Parse(string(src))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if len(outs) > 0 {
		// The grammar guarantees at least one Output, so -o always
		// REPLACES rather than rescues.
		t.Outputs = append([]string(nil), outs...)
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

// runValidate parses each tape and prints its compatibility warnings
// without recording anything: the CI lint for VHS migrations.
func runValidate(args []string, stdin io.Reader, stderr io.Writer) int {
	fs := flag.NewFlagSet("foley validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "deterministic",
		"clock the mode-gated warnings are linted for (deterministic|realtime)")
	mok := fs.Bool("modify-other-keys", false,
		"lint chords as if -modify-other-keys were set")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "usage: foley validate [flags] <file.tape ... | ->\n\n"+
			"Parses each tape and prints its compatibility warnings without\n"+
			"recording. Exits 1 if any tape fails to parse; warnings alone\n"+
			"exit 0.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}
	m, ok := parseMode(*mode)
	if !ok {
		_, _ = fmt.Fprintf(stderr, "foley: unknown mode %q (deterministic|realtime)\n", *mode)
		return 2
	}
	opts := tape.RunOptions{Mode: m, ModifyOtherKeys: *mok}
	exit := 0
	for _, arg := range fs.Args() {
		src, err := readTape(arg, stdin)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
			exit = 1
			continue
		}
		t, err := tape.Parse(string(src))
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", arg, err)
			exit = 1
			continue
		}
		for _, w := range tape.Lint(t, opts) {
			_, _ = fmt.Fprintf(stderr, "%s: warning: %s\n", arg, w)
		}
	}
	return exit
}

// runThemes lists the vendored theme catalog, one name per line.
func runThemes(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: foley themes")
		return 2
	}
	names, err := tape.Themes()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
		return 1
	}
	for _, n := range names {
		_, _ = fmt.Fprintln(stdout, n)
	}
	return 0
}
