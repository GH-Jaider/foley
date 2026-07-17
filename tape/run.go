package tape

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/key"
)

// RunOptions configures an execution. The zero value records in
// Deterministic mode with foley's font resolution and collects warnings
// silently.
type RunOptions struct {
	// Mode selects the clock. VHS has no realtime; Deterministic is both
	// the default and the faithful choice.
	Mode foley.Mode
	// ModifyOtherKeys forwards to foley.Options.ModifyOtherKeys: modern
	// CSI-27 encodings for modified keys instead of the xterm.js-parity
	// degradation. The executor points at this knob when a tape uses an
	// affected chord.
	ModifyOtherKeys bool
	// FontsDir forwards to foley.Options.FontsDir.
	FontsDir string
	// Dress REPLACES the tape's dress layer (ADR-014). Zero keeps the
	// tape's own `# foley: dress` cue; DressRef{None: true} strips the
	// layer. Explicit `Set`s in the tape always win over either. Build
	// one from CLI-style input with ParseDressRef.
	Dress DressRef
	// Warn receives one line per compatibility warning as it happens
	// (they are also collected in the Report). nil discards the stream.
	Warn io.Writer
}

// Report is what an execution produced.
type Report struct {
	Warnings []string
	Outputs  []string
}

// restlessWarnThreshold: how many restless settles (app output nobody
// asked for) earn the deterministic-mode hint. One is already reliable
// evidence — the driver exempts the launch paint and everything a
// keystroke prompted — so the only tapes below it are truly quiet ones.
const restlessWarnThreshold = 1

// Run executes a parsed tape end to end: resolves shell and theme,
// records every action against a foley Recorder, and encodes each
// declared Output. Relative paths (outputs, screenshots) resolve against
// the current working directory — run from the tape's directory, exactly
// like VHS. Compatibility gaps follow ADR-008: parsed always, executed
// faithfully or warned LOUDLY, never silent.
func Run(ctx context.Context, t *Tape, opts RunOptions) (*Report, error) {
	rep := &Report{}
	warn := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		rep.Warnings = append(rep.Warnings, msg)
		if opts.Warn != nil {
			_, _ = fmt.Fprintln(opts.Warn, "tape: warning:", msg)
		}
	}
	for _, msg := range Lint(t, opts) {
		warn("%s", msg)
	}

	settings, err := effectiveSettings(t, opts)
	if err != nil {
		return rep, err
	}

	for _, prog := range t.Requires {
		if _, err := execx.LookPath(prog); err != nil {
			return rep, fmt.Errorf("tape: Require %s: %w", prog, err)
		}
	}

	sh, err := shellFor(settings.Shell)
	if err != nil {
		return rep, err
	}
	if _, err := execx.LookPath(sh.command[0]); err != nil {
		return rep, fmt.Errorf("tape: Set Shell %s: %w", settings.Shell, err)
	}
	// Neutral frame: the effective ThemeRef may come from Set Theme OR a
	// dress (though dress themes were already validated at parse).
	theme, err := resolveTheme(settings.Theme)
	if err != nil {
		return rep, fmt.Errorf("tape: theme: %w", err)
	}

	env := append(os.Environ(), sh.env...)
	for k, v := range t.Env {
		env = append(env, k+"="+v)
	}

	bar, err := windowBarFor(settings.WindowBar)
	if err != nil {
		return rep, err
	}
	rec, err := foley.New(foley.Options{
		Command:         sh.command,
		Env:             env,
		PixelWidth:      settings.Width,
		PixelHeight:     settings.Height,
		PixelPadding:    settings.Padding,
		Margin:          settings.Margin,
		MarginFill:      settings.MarginFill,
		WindowBar:       bar,
		WindowBarSize:   settings.WindowBarSize,
		WindowBarColor:  settings.WindowBarColor,
		WindowTitle:     settings.WindowTitle,
		WindowTitleLeft: settings.TitleAlign == "left",
		BorderRadius:    settings.BorderRadius,
		FontSize:        settings.FontSize,
		Theme:           theme,
		FontsDir:        opts.FontsDir,
		Mode:            opts.Mode,
		FPS:             settings.Framerate,
		ModifyOtherKeys: opts.ModifyOtherKeys,
	})
	if err != nil {
		return rep, err
	}
	defer func() { _ = rec.Close() }()

	// PlaybackSpeed scales the recording: speed 2 halves every declared
	// duration (the video plays twice as fast). Wall-clock waits are
	// synchronization and stay unscaled.
	scale := func(d time.Duration) time.Duration {
		if settings.PlaybackSpeed == 1.0 || settings.PlaybackSpeed <= 0 {
			return d
		}
		return time.Duration(float64(d) / settings.PlaybackSpeed)
	}
	// perKey honors an EXPLICIT @duration even at zero — `Type@0ms` is
	// VHS's paste semantics, not "use the default".
	perKey := func(cmd Command) time.Duration {
		if cmd.SpeedSet {
			return scale(cmd.Speed)
		}
		return scale(settings.TypingSpeed)
	}

	var clipboard string
	for _, cmd := range t.Commands {
		var err error
		switch cmd.Kind {
		case KindType:
			err = rec.Type(ctx, cmd.Text, perKey(cmd))
		case KindPress:
			for i := 0; i < cmd.Count && err == nil; i++ {
				err = rec.Press(ctx, cmd.Key, perKey(cmd))
			}
		case KindSleep:
			err = rec.Sleep(ctx, scale(cmd.Speed))
		case KindWait:
			pattern := cmd.Pattern
			if pattern == nil {
				pattern = settings.WaitPattern
			}
			timeout := cmd.Timeout
			if timeout == 0 {
				timeout = settings.WaitTimeout
			}
			if cmd.Scope == WaitScreen {
				err = rec.WaitText(ctx, pattern, timeout)
			} else {
				err = rec.WaitLine(ctx, pattern, timeout)
			}
		case KindHide:
			err = rec.Hide()
		case KindShow:
			err = rec.Show()
		case KindScreenshot:
			err = rec.Screenshot(cmd.Text)
		case KindCopy:
			clipboard = cmd.Text
		case KindPaste:
			err = rec.Type(ctx, clipboard, 0)
		case KindScrollUp, KindScrollDown:
			warn("Scroll: mouse-wheel input is staged; the command was skipped (PageUp/PageDown work today)")
		}
		if err != nil {
			return rep, err
		}
	}

	// The question every animated-TUI tape raises, answered proactively.
	// One restless settle is already proof the app writes on its own —
	// launch paint and answered keystrokes never count — and the
	// keyframe collapse it warns about applies from the first one.
	if opts.Mode == foley.Deterministic && rec.RestlessSettles() >= restlessWarnThreshold {
		warn("the app wrote output on its own %d time(s) (animation or background activity); deterministic mode records settled keyframes only — run with --mode realtime to capture that motion as it happened", rec.RestlessSettles())
	}

	for _, out := range t.Outputs {
		if err := rec.Output(ctx, out); err != nil {
			return rep, fmt.Errorf("tape: Output %s: %w", out, err)
		}
		rep.Outputs = append(rep.Outputs, out)
	}
	return rep, rec.Close()
}

// effectiveSettings resolves the settings ONE run records with:
// defaults < dress (the tape's cue, or opts.Dress which REPLACES that
// layer) < the tape's explicit Sets — computed on a COPY, so Run never
// mutates the caller's Tape (parse once, run many: light/dark pairs).
func effectiveSettings(t *Tape, opts RunOptions) (Settings, error) {
	settings := t.Settings
	ref := t.DressCue()
	if !opts.Dress.IsZero() {
		ref = opts.Dress
	}
	if ref.IsZero() || ref.None {
		return settings, nil
	}
	d, err := ResolveDress(ref)
	if err != nil {
		return settings, err
	}
	explicit := make(map[string]bool, len(t.Explicit))
	for _, n := range t.Explicit {
		explicit[n] = true
	}
	applyDress(&settings, explicit, d)
	return settings, nil
}

// warnStaged emits the ADR-008 tier-2/3 warnings — only for settings the
// tape explicitly asked for.
func warnStaged(t *Tape, mode foley.Mode, warn func(string, ...any)) {
	for _, name := range t.Explicit {
		switch name {
		case "FontFamily":
			warn("Set FontFamily %q: foley pins JetBrains Mono for determinism; the requested font is ignored", t.Settings.FontFamily)
		case "LetterSpacing", "LineHeight":
			warn("Set %s: typographic metrics are staged raster work; the font's own metrics are used", name)
		case "CursorBlink":
			warn("Set CursorBlink: blinking is staged driver work; the cursor renders solid")
		case "LoopOffset":
			warn("Set LoopOffset: staged encode work; the GIF starts at the first frame")
		case "Framerate":
			if mode == foley.Deterministic {
				warn("Set Framerate: deterministic mode emits exact frames per state change; Framerate applies in realtime mode")
			}
		}
	}
	for _, name := range t.LateSets {
		warn("Set %s after the first action: VHS applies settings before recording starts; it was applied at the start anyway", name)
	}
}

// warnDegradedChords makes the ModifyOtherKeys choice VISIBLE: any chord
// that legacy terminals degrade (Ctrl/Alt on Enter/Tab/Space/Backspace)
// gets one warning naming the exact behavior and where to change it.
func warnDegradedChords(t *Tape, warn func(string, ...any)) {
	seen := map[string]bool{}
	for _, cmd := range t.Commands {
		if cmd.Kind != KindPress || cmd.Key.Mods&(key.ModCtrl|key.ModAlt) == 0 {
			continue
		}
		var base string
		switch cmd.Key.Name { //nolint:exhaustive // only the degradable named keys matter
		case key.NameEnter:
			base = "Enter"
		case key.NameTab:
			base = "Tab"
		case key.NameSpace:
			base = "Space"
		case key.NameBackspace:
			base = "Backspace"
		default:
			continue
		}
		chord := chordName(cmd.Key.Mods) + "+" + base
		if seen[chord] {
			continue
		}
		seen[chord] = true
		warn("%s: legacy apps receive a plain %s, exactly as in VHS/xterm; for the modern CSI-27 encoding set foley.Options.ModifyOtherKeys (CLI: --modify-other-keys)", chord, base)
	}
}

func chordName(m key.Mod) string {
	var parts []string
	if m&key.ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if m&key.ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if m&key.ModShift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(parts, "+")
}

// windowBarFor maps VHS's WindowBar names to the typed style. An
// unknown value is a LOUD error — never a silently bare window.
func windowBarFor(name string) (foley.WindowBarStyle, error) {
	switch name {
	case "":
		return foley.WindowBarNone, nil
	case "Colorful":
		return foley.WindowBarColorful, nil
	case "ColorfulRight":
		return foley.WindowBarColorfulRight, nil
	case "Rings":
		return foley.WindowBarRings, nil
	case "RingsRight":
		return foley.WindowBarRingsRight, nil
	case "LinuxControls":
		return foley.WindowBarLinuxControls, nil
	case "GnomeCSD":
		return foley.WindowBarGnomeCSD, nil
	}
	return foley.WindowBarNone, fmt.Errorf("tape: WindowBar %q: unknown style (Colorful|ColorfulRight|Rings|RingsRight|LinuxControls|GnomeCSD)", name)
}
