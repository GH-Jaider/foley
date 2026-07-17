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
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/key"
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
		case "doctor":
			return runDoctor(args[1:], stdout, stderr)
		case "new":
			return runNew(args[1:], stdout, stderr)
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
			"       foley new <file.tape>\n"+
			"       foley doctor [-fonts dir]\n"+
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
		switch fs.Arg(0) {
		case "validate", "themes", "doctor", "new":
			_, _ = fmt.Fprintf(stderr, "foley: (did you mean `foley %s …`? subcommands go before flags)\n", fs.Arg(0))
		}
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

// runDoctor verifies the recording toolchain end to end with ZERO
// system mutations: the pinned fonts load (hash-verified), the embedded
// engine records a 2-second smoke through a real pty, and ffmpeg meets
// the table minimum. Exit 0 means the full VHS workflow will work.
func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("foley doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fonts := fs.String("fonts", "",
		"directory holding the pinned fonts (default: $FOLEY_FONTS, then ./fonts)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "usage: foley doctor [-fonts dir]\n\n"+
			"Checks fonts, engine, a real 2s smoke recording and ffmpeg.\n"+
			"Prints findings and install hints; changes nothing.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}

	ok := true
	// Ctrl-C must run the deferred cleanups (staging dirs, child, pty)
	// instead of leaving litter — doctor promises "changes nothing".
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The version probe is bounded: a wedged ffmpeg must not hang doctor.
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	path, ffErr := execx.Find(probeCtx, execx.FFmpeg)
	cancel()
	if ffErr != nil {
		ok = false
		_, _ = fmt.Fprintf(stdout, "✗ ffmpeg: %v\n", ffErr)
		hint := "apt-get install ffmpeg (or your distro's package manager)"
		if runtime.GOOS == "darwin" {
			hint = "brew install ffmpeg"
		}
		_, _ = fmt.Fprintf(stdout, "  gif/mp4/webm outputs need it — install: %s\n", hint)
	} else {
		// No verification claim: execx deliberately passes unparseable
		// version strings (git builds), so "verified" would sometimes lie.
		_, _ = fmt.Fprintf(stdout, "✓ ffmpeg: %s\n", path)
	}

	if err := doctorSmoke(ctx, *fonts, stdout); err != nil {
		ok = false
		_, _ = fmt.Fprintf(stdout, "✗ record: %v\n", err)
	}

	if !ok {
		_, _ = fmt.Fprintln(stdout, "doctor: NOT ready — fix the ✗ items above")
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "doctor: ready — record something!")
	return 0
}

// doctorSmoke records 2 declared seconds against /bin/sh and renders
// frames to a throwaway dir: fonts (hash-verified by New), engine, pty,
// driver and rasterizer all prove themselves in one pass.
func doctorSmoke(ctx context.Context, fontsDir string, stdout io.Writer) error {
	rec, err := foley.New(foley.Options{
		Command:  []string{"/bin/sh"},
		Cols:     40,
		Rows:     8,
		FontsDir: fontsDir,
	})
	if err != nil {
		return err
	}
	defer func() { _ = rec.Close() }()
	if err := rec.Type(ctx, "echo doctor-ok", 0); err != nil {
		return err
	}
	if err := rec.Press(ctx, key.Named(key.NameEnter), 0); err != nil {
		return err
	}
	// Anchored to a line of its own: the pty ECHO of the typed command
	// already contains "doctor-ok", so an unanchored match would pass
	// before the shell executed anything.
	if err := rec.WaitText(ctx, regexp.MustCompile(`(?m)^doctor-ok$`), 10*time.Second); err != nil {
		return err
	}
	if err := rec.Sleep(ctx, 2*time.Second); err != nil {
		return err
	}
	// MkdirTemp, not a predictable name: a guessable path in shared /tmp
	// is a symlink-redirection surface, and a stale dir from a killed run
	// would pollute the frame count below.
	dir, err := os.MkdirTemp("", "foley-doctor-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := rec.Output(ctx, filepath.Join(dir, "frames")); err != nil {
		return err
	}
	frames, err := filepath.Glob(filepath.Join(dir, "frames", "*.png"))
	if err != nil {
		return err
	}
	if len(frames) == 0 {
		return errors.New("smoke rendered no frames")
	}
	_, _ = fmt.Fprintf(stdout, "✓ record: fonts verified, engine up, 2s smoke → %d frame(s)\n", len(frames))
	return nil
}

// runNew scaffolds a starter tape, VHS's `vhs new` parity. Creation is
// atomic (O_EXCL): it never overwrites — not through a race, not through
// a dangling symlink.
func runNew(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("foley new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "usage: foley new <file.tape>\n\n"+
			"Writes a starter tape (\".tape\" is appended if missing, parent\n"+
			"directories are created). Never overwrites an existing file.\n")
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
	path := fs.Arg(0)
	if path == "-" {
		_, _ = fmt.Fprintln(stderr, `foley: new cannot write to "-" (that is the CLI's stdin convention)`)
		return 2
	}
	if !strings.HasSuffix(path, ".tape") {
		path += ".tape"
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
			return 1
		}
	}
	base := strings.TrimSuffix(filepath.Base(path), ".tape")
	tapeSrc := fmt.Sprintf(`# %s.tape — recorded with foley (VHS-compatible)
#   foley %s

Output %s.gif

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 600
Set Padding 40
Set TypingSpeed 50ms

Type "echo hola, foley"
Enter
Sleep 2s
`, base, path, base)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // the target path is the CLI's whole purpose; a tape is a public artifact
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			_, _ = fmt.Fprintf(stderr, "foley: %s already exists — refusing to overwrite\n", path)
		} else {
			_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
		}
		return 1
	}
	if _, err := f.WriteString(tapeSrc); err != nil {
		_ = f.Close()
		_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
		return 1
	}
	if err := f.Close(); err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "foley: wrote %s — record it with: foley %s\n", path, path)
	return 0
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
