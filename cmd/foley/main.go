// Command foley renders a VHS .tape script into gif/mp4/webm/webp/cast/txt/frames
// without a terminal window: the demo runs on a real pty against an
// embedded terminal engine and foley rasterizes every frame itself. It
// is a thin consumer of the public API (library first): flags in,
// tape.Run out. Besides recording, `foley validate` lints tapes without
// running them and `foley themes` lists the vendored theme catalog.
package main

import (
	"context"
	"encoding/json"
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
	"unicode"

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

// repeatFlag collects a repeatable string flag (-o, -env).
type repeatFlag []string

func (o *repeatFlag) String() string { return strings.Join(*o, ", ") }
func (o *repeatFlag) Set(v string) error {
	*o = append(*o, v)
	return nil
}

// parseKeysOverride maps the -keys flag values shared by record and
// play; false means an unknown value (the caller prints the menu).
func parseKeysOverride(s string) (tape.KeysOverride, error) {
	if s == "" {
		return tape.KeysOverride{}, nil
	}
	if s == "off" {
		return tape.KeysOverride{Off: true}, nil
	}
	// Commas or spaces ("on,large" or a quoted "on large") both speak
	// the cue's own token grammar — one grammar, two doors. A leading
	// "on" is optional: any styling token implies the reel is wanted.
	toks := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	if len(toks) > 0 && toks[0] == "on" {
		toks = toks[1:]
	}
	kc, err := tape.ParseKeysArgs(strings.Join(toks, " "))
	if err != nil {
		return tape.KeysOverride{}, err
	}
	return tape.KeysOverride{On: true, Cue: kc}, nil
}

// parseInterleaved parses flags and positional arguments in ANY order:
// `foley demo.tape -watch` works exactly like `foley -watch demo.tape`.
// Go's flag package stops at the first non-flag token; this collects
// that token and resumes parsing, until the arguments run out. The
// positional arguments come back in their original order.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
}

// flagGroups is the curated `foley -h` layout: every record flag lives
// in a group, in reading order — the alphabetical soup of PrintDefaults
// is exactly what a reference card must not be. A flag missing from
// this table prints under a LOUD "ungrouped" header (nothing silent),
// and TestDashHIsGrouped pins that the header never ships.
//
//nolint:gochecknoglobals // immutable help layout
var flagGroups = []struct {
	title string
	names []string
}{
	{"The take", []string{"mode", "studio", "dir", "env", "cols", "rows"}},
	{"The look", []string{"dress", "keys", "theme"}},
	{"The output", []string{"o", "output-scale", "gif-loop"}},
	{"The loop", []string{"watch"}},
	{"System", []string{"fonts", "modify-other-keys", "version"}},
}

// printGroupedFlags renders the record flags under flagGroups' headers.
func printGroupedFlags(w io.Writer, fs *flag.FlagSet, st styles) {
	seen := map[string]bool{}
	for _, g := range flagGroups {
		_, _ = fmt.Fprintf(w, "%s\n", st.head.Render(g.title+":"))
		for _, name := range g.names {
			if f := fs.Lookup(name); f != nil {
				seen[name] = true
				printFlag(w, f, st)
			}
		}
		_, _ = fmt.Fprintln(w)
	}
	headed := false
	fs.VisitAll(func(f *flag.Flag) {
		if seen[f.Name] {
			return
		}
		if !headed {
			headed = true
			_, _ = fmt.Fprintln(w, "ungrouped (add to flagGroups):")
		}
		printFlag(w, f, st)
	})
}

// printFlag renders one flag PrintDefaults-style, minus the sorting.
func printFlag(w io.Writer, f *flag.Flag, st styles) {
	arg, usage := flag.UnquoteUsage(f)
	head := "  -" + f.Name
	if arg != "" {
		head += " " + arg
	}
	if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
		usage += fmt.Sprintf(" (default %s)", f.DefValue)
	}
	_, _ = fmt.Fprintln(w, st.accent.Render(head))
	_, _ = fmt.Fprintln(w, "        "+usage)
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
		case "manual":
			return runManual(args[1:], stdout, stderr)
		case "new":
			return runNew(args[1:], stdout, stderr)
		case "wardrobe":
			return runWardrobe(args[1:], stdout, stderr)
		case "fonts":
			return runFonts(args[1:], stdout, stderr)
		case "sew":
			return runSew(args[1:], stdout, stderr)
		case "play":
			return runPlay(args[1:], stdin, stdout, stderr)
		case "skill":
			return runSkill(args[1:], stdout, stderr)
		case "completion":
			return runCompletion(args[1:], stdout, stderr)
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
	var outs repeatFlag
	fs.Var(&outs, "o",
		"write the recording to this path (repeatable; format by extension, replaces the tape's Output declarations)")
	dress := fs.String("dress", "",
		"replace the tape's dress layer: a built-in name (list: foley wardrobe), a .json path, an inline {json}, or none — the tape's explicit Sets still win")
	keys := fs.String("keys", "",
		"replace the tape's keys layer: off, or keys tokens separated by commas or spaces — on, small|medium|large, notation=keycap|icons, accent=<ansi|#hex|off>, plain (default: the tape's keys cue decides)")
	watch := fs.Bool("watch", false,
		"re-record every time the tape (or a Source'd tape or dress file it uses) is saved; ctrl-c stops")
	themeFlag := fs.String("theme", "",
		"replace the recording's theme (a curated name or an inline {json}) — explicit Set Theme included: record dark/light pairs from one tape")
	outputScale := fs.Int("output-scale", 2,
		"output resolution: 2 = retina (the default), 1 = logical size (about a quarter of the file weight; hairlines soften)")
	dirFlag := fs.String("dir", "",
		"working directory for the tape's COMMAND only (default: the current directory) — output paths still resolve against your cwd, like VHS")
	studio := fs.Bool("studio", false,
		"build a closed set for the take: a fresh directory becomes HOME, the working directory and every temp default, struck when the recording ends — your machine stays off camera (the tape's '# foley: studio' cue does the same)")
	var extraEnv repeatFlag
	fs.Var(&extraEnv, "env",
		"add KEY=VALUE to the recording's environment without writing it into the tape (repeatable; wins over the tape's Env)")
	gifLoop := fs.Int("gif-loop", 0,
		"gif loop count: 0 = forever (the default), -1 = play once, N = repeat N more times")
	cols := fs.Int("cols", 0, "terminal grid columns (overrides the tape's pixel Width/Height derivation)")
	rows := fs.Int("rows", 0, "terminal grid rows (overrides the tape's pixel Width/Height derivation)")
	// Three doors, three sizes: bare `foley` is a WELCOME (short, on
	// stdout, exit 0 — someone exploring got their answer); a malformed
	// invocation gets the compact usage on stderr; -h gets the full
	// flag reference. The full dump is a reference card, not a front
	// door — it does not fit on a screen and should not have to.
	pitch := "foley — terminal demos from .tape scripts, rendered without a window.\n" +
		"The tape is the recording; foley adds the post-production (camera,\n" +
		"highlights, the keys reel) and the footage is never touched.\n"
	usageLines := "usage: foley [flags] <file.tape | ->\n" +
		"       foley play [flags] <file.tape | ->\n" +
		"       foley validate [flags] <file.tape ... | ->\n" +
		"       foley new <file.tape>\n" +
		"       foley sew [-from <dress>] <name>\n" +
		"       foley doctor [-fonts dir]\n" +
		"       foley themes | fonts | wardrobe [name]\n" +
		"       foley manual\n" +
		"       foley skill\n" +
		"       foley completion bash|zsh|fish\n"
	welcome := func(w io.Writer) {
		showLogo(w)
		st := newStyles(w)
		var b strings.Builder
		b.WriteString(pitch + "\n" + st.head.Render("start here:") + "\n")
		for _, c := range [...]struct{ cmd, what string }{
			{"foley new demo.tape", "a starter tape to edit"},
			{"foley demo.tape", "record its outputs (gif/mp4/webm/webp/cast/txt)"},
			{"foley play demo.tape", "watch it right here, in this terminal"},
			{"foley -watch demo.tape", "re-record every time you save"},
			{"foley validate demo.tape", "the spotting session: lint + cue sheet"},
			{"foley doctor", "check fonts, engine and ffmpeg"},
			{"foley manual", "the manual, right here in the terminal"},
			{"foley skill", "the same knowledge for an AI agent, as one file"},
		} {
			b.WriteString("  " + st.accent.Render(fmt.Sprintf("%-25s", c.cmd)) + "  " + c.what + "\n")
		}
		b.WriteString("\nalso: sew (make a dress) · themes · fonts · wardrobe · completion\n")
		b.WriteString("flags: " + st.accent.Render("foley -h") + "              the full reference\n")
		_, _ = fmt.Fprint(w, b.String())
	}
	fullHelp := func(w io.Writer) {
		_, _ = fmt.Fprint(w, pitch+"\n"+usageLines+"\n"+
			"\"-\" reads the tape from stdin. Relative paths in the tape (Output,\n"+
			"Screenshot, Source) resolve against the current working directory,\n"+
			"exactly like VHS. Flags go before or after the tape path — both work.\n\n")
		printGroupedFlags(w, fs, newStyles(w))
	}
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, usageLines+"\nfoley -h lists every flag\n")
	}
	if len(args) == 0 {
		welcome(stdout)
		return 0
	}
	for _, a := range args {
		if a == "-h" || a == "-help" || a == "--help" {
			fullHelp(stdout)
			return 0
		}
	}
	files, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		_, _ = fmt.Fprintln(stdout, "foley "+versionString())
		return 0
	}
	if len(files) != 1 {
		fs.Usage()
		return 2
	}
	tapeArg := files[0]
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
	if *outputScale != 1 && *outputScale != 2 {
		_, _ = fmt.Fprintf(stderr, "foley: -output-scale %d: 2 is the retina default, 1 halves to logical size\n", *outputScale)
		return 2
	}
	if *studio && *dirFlag != "" {
		_, _ = fmt.Fprintln(stderr, "foley: -studio and -dir contradict each other — the studio builds its own working directory")
		return 2
	}

	// The pulse: a live progress line on stderr while recording and
	// developing — only when stderr is a terminal; CI and pipes stay
	// silent as always.
	progress := newProgressRenderer(stderr)
	// One style probe per invocation: building styles interrogates the
	// terminal (background query), so recordOnce must not re-probe per
	// output — and -watch re-records forever.
	st := newStyles(stdout)
	opts := tape.RunOptions{
		Mode:            m,
		ModifyOtherKeys: *mok,
		FontsDir:        *fonts,
		Dress:           dressRef,
		Keys:            keysOverride,
		Theme:           themeRef,
		OutputScale:     *outputScale,
		Dir:             *dirFlag,
		Studio:          *studio,
		ExtraEnv:        extraEnv,
		GIFLoop:         *gifLoop,
		Cols:            *cols,
		Rows:            *rows,
		Warn:            progress.warnWriter(),
		Progress:        progress.pulse,
	}
	// Ctrl-C cancels the context: actions fail fast and the Recorder's
	// cleanup (process, engine, staging) runs instead of leaking.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// recordOnce runs the whole read→parse→record flow, printing its
	// own errors. It reports the files this tape pulls in (for -watch)
	// and the exit code.
	recordOnce := func() (watchPaths []string, exit int) {
		watchPaths = []string{tapeArg}
		src, err := readTape(tapeArg, stdin)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
			switch tapeArg {
			case "play", "validate", "themes", "doctor", "manual", "skill", "new", "sew", "fonts", "wardrobe", "completion":
				_, _ = fmt.Fprintf(stderr, "foley: (did you mean `foley %s …`? subcommands go before flags)\n", tapeArg)
			}
			return watchPaths, 1
		}
		t, err := tape.Parse(string(src))
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%v\n", err)
			return watchPaths, 1
		}
		watchPaths = append(watchPaths, tape.SourcedTapes(string(src))...)
		if p := t.DressCue().Path; p != "" {
			watchPaths = append(watchPaths, p)
		}
		if len(outs) > 0 {
			// The grammar guarantees at least one Output, so -o always
			// REPLACES rather than rescues.
			t.Outputs = append([]string(nil), outs...)
		}
		rep, err := tape.Run(ctx, t, opts)
		progress.done()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
			return watchPaths, 1
		}
		for _, out := range rep.Outputs {
			_, _ = fmt.Fprintf(stdout, "foley: wrote %s\n", st.accent.Render(out))
		}
		return watchPaths, 0
	}

	if *watch {
		if tapeArg == "-" {
			_, _ = fmt.Fprintln(stderr, "foley: -watch needs a file path (stdin has nothing to watch)")
			return 2
		}
		watchLoop(ctx, stderr, watchPoll, tapeArg, func() []string {
			paths, _ := recordOnce() // a broken tape keeps the watch alive
			return paths
		})
		return 0
	}
	_, exit := recordOnce()
	return exit
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
			"The spotting session: parses each tape, reports compatibility\n"+
			"warnings and its cue sheet — nothing records. Exits 1 if any tape\n"+
			"fails to parse; warnings alone exit 0.\n\n")
		fs.PrintDefaults()
	}
	files, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if len(files) == 0 {
		fs.Usage()
		return 2
	}
	m, ok := parseMode(*mode)
	if !ok {
		_, _ = fmt.Fprintf(stderr, "foley: unknown mode %q (deterministic|realtime)\n", *mode)
		return 2
	}
	opts := tape.RunOptions{Mode: m, ModifyOtherKeys: *mok}
	st := newStyles(stderr)
	exit := 0
	for _, arg := range files {
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
			_, _ = fmt.Fprintf(stderr, "%s: %s %s\n", arg, st.warn.Render("warning:"), w)
		}
		if n := len(t.Cues); n > 0 {
			dresses, keys, highlights, zooms, studios := 0, 0, 0, 0, 0
			for _, c := range t.Cues {
				switch c.Kind {
				case tape.CueDress:
					dresses++
				case tape.CueKeys:
					keys++
				case tape.CueHighlight:
					highlights++
				case tape.CueZoom:
					zooms++
				case tape.CueStudio:
					studios++
				}
			}
			plural := "s"
			if n == 1 {
				plural = ""
			}
			_, _ = fmt.Fprintf(stderr, "%s: %s %d cue%s — %d dress, %d keys, %d highlight, %d zoom, %d studio\n", arg, st.accent.Render("cue sheet:"), n, plural, dresses, keys, highlights, zooms, studios)
			if zooms > 0 {
				_, _ = fmt.Fprintf(stderr, "%s: %s\n", arg, st.dim.Render("note: the zoom sharp-cap check needs the real font geometry — it runs at frame zero of the recording, before any key is typed"))
			}
		}
		// The spotting session signs off out loud: silence reads as
		// "did nothing" to humans and agents alike (found live by an
		// agent double-checking an empty validate). Parse failures
		// already spoke and continued before reaching here.
		_, _ = fmt.Fprintf(stderr, "%s: %s commands: %d · cues: %d · outputs: %s\n",
			arg, st.ok.Render("ok —"), len(t.Commands), len(t.Cues), strings.Join(t.Outputs, ", "))
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
	st := newStyles(stdout)
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
		_, _ = fmt.Fprintf(stdout, "%s ffmpeg: %v\n", st.bad.Render("✗"), ffErr)
		hint := "apt-get install ffmpeg (or your distro's package manager)"
		if runtime.GOOS == "darwin" {
			hint = "brew install ffmpeg"
		}
		_, _ = fmt.Fprintf(stdout, "  %s\n", st.dim.Render("gif/mp4/webm/webp outputs need it — install: "+hint))
	} else {
		// No verification claim: execx deliberately passes unparseable
		// version strings (git builds), so "verified" would sometimes lie.
		_, _ = fmt.Fprintf(stdout, "%s ffmpeg: %s\n", st.ok.Render("✓"), path)
	}

	if err := doctorSmoke(ctx, *fonts, stdout, st); err != nil {
		ok = false
		_, _ = fmt.Fprintf(stdout, "%s record: %v\n", st.bad.Render("✗"), err)
	}

	if !ok {
		_, _ = fmt.Fprintln(stdout, "doctor: "+st.bad.Render("NOT ready")+" — fix the ✗ items above")
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "doctor: "+st.ok.Render("ready")+" — record something!")
	return 0
}

// doctorSmoke records 2 declared seconds against /bin/sh and renders
// frames to a throwaway dir: fonts (hash-verified by New), engine, pty,
// driver and rasterizer all prove themselves in one pass.
func doctorSmoke(ctx context.Context, fontsDir string, stdout io.Writer, st styles) error {
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
	_, _ = fmt.Fprintf(stdout, "%s record: fonts verified, engine up, 2s smoke → %d frame(s)\n", st.ok.Render("✓"), len(frames))
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
	st := newStyles(stdout)
	_, _ = fmt.Fprintf(stdout, "foley: wrote %s — record it with: %s\n", st.accent.Render(path), st.accent.Render("foley "+path))
	return 0
}

// userDressDir is the CLI-side wardrobe (~/.config/foley/dresses). It
// resolves NAMES only for CLI flags — tapes stay self-contained (their
// dresses are built-ins, ./paths or inline).
func userDressDir() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cfg, "foley", "dresses")
}

// resolveDressArg upgrades a bare -dress NAME that is not a built-in to
// the user wardrobe path when that file exists; everything else passes
// through untouched (paths, inline JSON, none, built-ins — built-ins
// always win a name clash, and wardrobe marks the shadow).
func resolveDressArg(arg string) string {
	if arg == "" || arg == "none" || strings.HasPrefix(arg, "{") ||
		strings.ContainsAny(arg, "/\\") || strings.HasSuffix(arg, ".json") {
		return arg
	}
	for _, b := range tape.BuiltinDresses() {
		if b == arg {
			return arg
		}
	}
	if dir := userDressDir(); dir != "" {
		p := filepath.Join(dir, arg+".json")
		if _, err := os.Stat(p); err == nil { //nolint:gosec // the user's own wardrobe dir + their own -dress name
			return p
		}
	}
	return arg
}

// runWardrobe lists the available dresses, or shows one dress's
// expansion into the Set primitives it fills.
func runWardrobe(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		_, _ = fmt.Fprint(stderr, "usage: foley wardrobe [name]\n\n"+
			"Lists the available dresses (built-ins plus ~/.config/foley/dresses),\n"+
			"or shows what one dress expands to. Name clashes resolve to the\n"+
			"built-in; shadowed files are marked.\n")
		return 0
	}
	builtins := map[string]bool{}
	for _, b := range tape.BuiltinDresses() {
		builtins[b] = true
	}
	st := newStyles(stdout)
	switch len(args) {
	case 0:
		for _, name := range tape.BuiltinDresses() {
			_, _ = fmt.Fprintf(stdout, "%s %s\n", name, st.dim.Render("(built-in)"))
		}
		dir := userDressDir()
		if dir == "" {
			return 0
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				_, _ = fmt.Fprintf(stderr, "foley: warning: wardrobe dir unreadable: %v\n", err)
			}
			return 0
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".json")
			mark := ""
			if builtins[name] {
				mark = st.warn.Render(" — SHADOWED by the built-in; rename it or use its path")
			}
			_, _ = fmt.Fprintf(stdout, "%s %s%s\n", name, st.dim.Render("(yours: "+filepath.Join(dir, e.Name())+")"), mark)
		}
		return 0
	case 1:
		spec := resolveDressArg(args[0])
		ref, err := tape.ParseDressRef(spec)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
			_, _ = fmt.Fprintf(stderr, "foley: (built-ins: %s; yours live in %s)\n",
				strings.Join(tape.BuiltinDresses(), ", "), userDressDir())
			return 1
		}
		d, err := tape.ResolveDress(ref)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: %v\n", err)
			return 1
		}
		lines := d.Expansion()
		if len(lines) == 0 {
			_, _ = fmt.Fprintf(stdout, "dress %s expands to nothing (empty dress)\n", args[0])
			return 0
		}
		_, _ = fmt.Fprintf(stdout, "dress %s expands to:\n", args[0])
		for _, l := range lines {
			_, _ = fmt.Fprintln(stdout, "  "+l)
		}
		return 0
	default:
		_, _ = fmt.Fprintln(stderr, "usage: foley wardrobe [name]")
		return 2
	}
}

// runThemes lists the vendored theme catalog, one name per line.
// sewTemplate is the starter dress `foley sew` writes without -from:
// every field filled with WORKING values (it validates and records as
// written — JSON has no comments, so the example is the documentation).
// The font field is deliberately absent: a path to a file that does not
// exist yet would break the first run; the next-steps print names it.
const sewTemplate = `{
  "theme": "Catppuccin Mocha",
  "fontSize": 16,
  "windowBar": "Colorful",
  "windowBarSize": 28,
  "windowTitle": "~",
  "titleAlign": "center",
  "borderRadius": 10,
  "margin": 0,
  "marginFill": "#101014",
  "padding": 30
}
`

// runSew scaffolds a dress file — the costume shop. `-from` copies an
// existing dress (built-in, path or user wardrobe) as the starting
// point. Never overwrites (O_EXCL), like `foley new`.
func runSew(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sew", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "",
		"start from an existing dress: a built-in (list: foley wardrobe), a .json path, or a name in ~/.config/foley/dresses")
	files, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if len(files) != 1 {
		_, _ = fmt.Fprint(stderr, "usage: foley sew [-from <dress>] <name>\n\n"+
			"Writes <name>.dress.json — a dress to edit and wear with a\n"+
			"`# foley: dress ./<name>.dress.json` cue (or -dress per run).\n")
		return 2
	}
	name := files[0]
	path := name
	if !strings.HasSuffix(path, ".json") {
		path = name + ".dress.json"
	}

	content := []byte(sewTemplate)
	if *from != "" {
		ref, err := tape.ParseDressRef(resolveDressArg(*from))
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: sew: -from: %v\n", err)
			return 1
		}
		d, err := tape.ResolveDress(ref)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: sew: -from: %v\n", err)
			return 1
		}
		raw, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "foley: sew: %v\n", err)
			return 1
		}
		content = append(raw, '\n')
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // the target path is the CLI's whole purpose; a dress is a public artifact
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: sew: %v (sew a new name, or edit the existing file)\n", err)
		return 1
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		_, _ = fmt.Fprintf(stderr, "foley: sew: %v\n", err)
		return 1
	}
	if err := f.Close(); err != nil {
		_, _ = fmt.Fprintf(stderr, "foley: sew: %v\n", err)
		return 1
	}
	st := newStyles(stdout)
	_, _ = fmt.Fprintf(stdout, "sewed %s\n\n", st.accent.Render(path))
	_, _ = fmt.Fprintf(stdout, "  wear it in a tape:  %s\n", st.accent.Render("# foley: dress ./"+path))
	_, _ = fmt.Fprintf(stdout, "  or per run:         %s\n\n", st.accent.Render("foley -dress ./"+path+" demo.tape"))
	_, _ = fmt.Fprint(stdout, st.dim.Render("  fields: theme (foley themes), font (foley fonts, ./file.ttf, or\n"+
		"  {regular/bold/italic/boldItalic} files), fontSize, windowBar,\n"+
		"  windowBarSize, windowBarColor, windowTitle, titleAlign, margin,\n"+
		"  marginFill, borderRadius, padding — delete what the dress\n"+
		"  should not touch; the tape's explicit Sets always win.")+"\n")
	return 0
}

// runFonts lists the pinned font family catalog — the names a
// `Set FontFamily` (or a dress `font`) resolves without touching the
// system. Your own fonts travel as ./file.ttf paths.
func runFonts(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: foley fonts")
		return 2
	}
	st := newStyles(stdout)
	for _, n := range foley.FontFamilies() {
		suffix := ""
		if n == foley.DefaultFontFamily {
			suffix = " " + st.dim.Render("(default)")
		}
		_, _ = fmt.Fprintln(stdout, n+suffix)
	}
	return 0
}

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
