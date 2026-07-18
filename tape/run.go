package tape

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/internal/execx"
	"github.com/GH-Jaider/foley/key"
)

// KeysOverride is the CLI's replacement for the tape's keys layer
// (ADR-016). The zero value defers to the tape.
type KeysOverride uint8

// The keys overrides.
const (
	KeysDefault KeysOverride = iota
	KeysOff
	KeysOnSmall
	KeysOnMedium
	KeysOnLarge
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
	// Keys REPLACES the tape's keys switch (ADR-016): the zero value
	// keeps the tape's own `# foley: keys` cue; the others force the
	// reel off or on at a size (the CLI's -keys off|on|small|medium|
	// large).
	Keys KeysOverride
	// Theme REPLACES the recording's theme — explicit Sets included,
	// unlike Dress: its whole purpose is recording the SAME tape in
	// two palettes (a dark/light pair) without editing it. Zero keeps
	// the tape's own theme. Build one with ParseThemeRef.
	Theme ThemeRef
	// Warn receives one line per compatibility warning as it happens
	// (they are also collected in the Report). nil discards the stream.
	Warn io.Writer
	// Progress, when non-nil, receives the recording's pulse: one event
	// before the first command, one after each command, and one before
	// each output is developed. Called synchronously from Run's
	// goroutine — a slow callback slows the recording; render cheap.
	Progress func(ProgressEvent)
	// KeepFrames preserves the staging frames (PNGs + manifest) past
	// the run; Report.FramesDir names the directory and the CALLER
	// owns deleting it. `foley play` replays it.
	KeepFrames bool
	// OutputScale forwards to foley.Options.OutputScale: 0/2 = the
	// retina default, 1 = halve to logical size (quarter the weight,
	// hairlines soften — weight versus crispness is the user's call).
	OutputScale int
	// Dir runs the tape's command in this working directory (#320's
	// ask, without touching the vendored grammar). Empty inherits.
	Dir string
	// ExtraEnv adds KEY=VALUE pairs to the child environment WITHOUT
	// writing them into the tape (#621: secrets and machine-local
	// values). They win over the tape's own Env. Prompt variables here
	// do NOT retune bare Wait — that derivation reads the tape.
	ExtraEnv []string
	// GIFLoop is ffmpeg's gif -loop value (#633): 0 = loop forever
	// (the default), -1 = play once, N = repeat N extra times.
	GIFLoop int
	// Cols, Rows size the grid directly (#578), overriding the tape's
	// pixel Width/Height derivation. Zero defers to the tape.
	Cols, Rows int
}

// ProgressPhase names what the recording is doing right now.
type ProgressPhase uint8

// The phases a Run pulses through.
const (
	// ProgressRecording: the script is executing; Now advances toward
	// Total (deterministic) or just advances (realtime).
	ProgressRecording ProgressPhase = iota
	// ProgressDeveloping: frames are being encoded into Output.
	ProgressDeveloping
)

// ProgressEvent is one pulse of a recording (RunOptions.Progress).
type ProgressEvent struct {
	Phase ProgressPhase
	// Now is the timeline position: virtual time in deterministic mode,
	// elapsed wall time in realtime.
	Now time.Duration
	// Total is the script's own promise: the sum of every declared
	// duration (typing, presses, sleeps) at their scaled speeds. Waits
	// and settles are synchronization — unknowable ahead — so Now can
	// legitimately arrive past Total. Zero in realtime: a camera
	// rolling on the wall clock has no declared end.
	Total time.Duration
	// Frames emitted so far.
	Frames int
	// Output is the file being developed (ProgressDeveloping only).
	Output string
}

// Report is what an execution produced.
type Report struct {
	Warnings []string
	Outputs  []string
	// FramesDir is the preserved staging directory (RunOptions.
	// KeepFrames). Set as soon as the recorder exists — even on a
	// failed run — because the caller owns cleaning it up.
	FramesDir string
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
	for _, kv := range opts.ExtraEnv {
		if k, _, _ := strings.Cut(kv, "="); k == "PS1" || k == "PROMPT" {
			warn("-env %s: prompt variables from the CLI do not retune bare Wait — put `Env %s` in the tape for a prompt-aware Wait", k, k)
		}
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
	if sh.generic {
		warn("Set Shell %s: not in foley's table (VHS's nine) — launched bare with its own defaults; the standard `>` Wait pattern will not match: Wait /regex/ explicitly or set Env PS1 if it honors it", settings.Shell)
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

	env := mergeEnv(os.Environ(), sh.env, envPairs(t.Env), opts.ExtraEnv)

	// A custom prompt (ADR-017): `Env PS1` was always legal grammar —
	// mergeEnv above is what makes it actually WIN over the shell
	// table. What remains is teaching bare `Wait` to expect the new
	// prompt. Lint (already run above) voiced the findings; here we
	// only take the resolved pattern.
	settings.WaitPattern = promptWaitPattern(t, settings, func(string, ...any) {})

	bar, err := windowBarFor(settings.WindowBar)
	if err != nil {
		return rep, err
	}
	// One zoom cue anywhere reserves the camera for the whole run
	// (ADR-019): the master is a per-recorder decision, made before the
	// first frame. Tapes without zoom cues never pay for it.
	zoomWanted := false
	for _, c := range t.Cues {
		if c.Kind == CueZoom {
			zoomWanted = true
			break
		}
	}
	// A .cast output needs the byte stream retained from frame zero.
	captureCast := false
	for _, out := range t.Outputs {
		if strings.EqualFold(filepath.Ext(out), ".cast") {
			captureCast = true
			break
		}
	}
	rec, err := foley.New(foley.Options{
		Command:         sh.command,
		Dir:             opts.Dir,
		Env:             env,
		Cols:            opts.Cols,
		Rows:            opts.Rows,
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
		KeysOverlay:     settings.KeysOverlay,
		KeysSize:        settings.KeysSize,
		BorderRadius:    settings.BorderRadius,
		FontSize:        settings.FontSize,
		FontFamily:      fontFamilyFor(settings.FontFamily),
		FontFile:        fontFileFor(settings.FontFamily),
		FontFiles: foley.FontFiles{
			Regular: settings.FontFiles.Regular, Bold: settings.FontFiles.Bold,
			Italic: settings.FontFiles.Italic, BoldItalic: settings.FontFiles.BoldItalic,
		},
		Theme:           theme,
		FontsDir:        opts.FontsDir,
		Mode:            opts.Mode,
		FPS:             settings.Framerate,
		ModifyOtherKeys: opts.ModifyOtherKeys,
		Zoom:            zoomWanted,
		KeepFrames:      opts.KeepFrames,
		CaptureCast:     captureCast,
		OutputScale:     opts.OutputScale,
		GIFLoop:         opts.GIFLoop,
	})
	if err != nil {
		return rep, err
	}
	defer func() { _ = rec.Close() }()
	if opts.KeepFrames {
		rep.FramesDir = rec.FramesDir()
	}
	for _, w := range rec.AssemblyWarnings() {
		warn("%s", w)
	}

	// PlaybackSpeed scales the recording: speed 2 halves every declared
	// duration (the video plays twice as fast). Wall-clock waits are
	// synchronization and stay unscaled.
	scale := func(d time.Duration) time.Duration { return scaleDur(settings, d) }
	// perKey honors an EXPLICIT @duration even at zero — `Type@0ms` is
	// VHS's paste semantics, not "use the default".
	perKey := func(cmd Command) time.Duration { return perKeyDur(settings, cmd) }
	// Camera transitions live on the same virtual timeline PlaybackSpeed
	// compresses, so they scale like Type and Sleep — the default too.
	zoomDur := func(d time.Duration) time.Duration {
		if d == 0 {
			d = foley.DefaultZoomDur
		}
		return scale(d)
	}

	// Pre-flight every zoom cue (ADR-019): the rect against the sharp
	// cap, the SCALED duration against the frame-count cap (a slow
	// PlaybackSpeed stretches transitions past what parse saw). A bad
	// zoom fails HERE, at frame zero, never mid-take.
	for _, c := range t.Cues {
		if c.Kind != CueZoom {
			continue
		}
		if !c.Zoom.Off {
			if err := rec.ZoomCheck(c.Zoom.Col, c.Zoom.Row, c.Zoom.W, c.Zoom.H); err != nil {
				return rep, fmt.Errorf("tape: %d: %w", c.Line, err)
			}
		}
		if d := zoomDur(c.Zoom.Dur); d > foley.MaxZoomDur {
			return rep, fmt.Errorf("tape: %d: zoom: PlaybackSpeed %g stretches the %v transition to %v — past the %v cap (each second renders ~30 physical frames)",
				c.Line, settings.PlaybackSpeed, c.Zoom.Dur, d, foley.MaxZoomDur)
		}
	}

	// Positional cues (highlight ADR-018, zoom ADR-019) fire at their
	// POSITION in the script: everything with AfterCommand <= i acts
	// before command i runs, stamped with the current virtual instant,
	// in tape order.
	var posCues []Cue
	for _, c := range t.Cues {
		if c.Kind == CueHighlight || c.Kind == CueZoom {
			posCues = append(posCues, c)
		}
	}
	nextCue := 0
	applyCues := func(i int) error {
		for nextCue < len(posCues) && posCues[nextCue].AfterCommand <= i {
			c := posCues[nextCue]
			nextCue++
			switch {
			case c.Kind == CueZoom && c.Zoom.Off:
				if err := rec.ZoomOff(zoomDur(c.Zoom.Dur)); err != nil {
					return fmt.Errorf("tape: %d: %w", c.Line, err)
				}
			case c.Kind == CueZoom:
				if err := rec.Zoom(c.Zoom.Col, c.Zoom.Row, c.Zoom.W, c.Zoom.H, zoomDur(c.Zoom.Dur)); err != nil {
					return fmt.Errorf("tape: %d: %w", c.Line, err)
				}
			case c.HighlightOff && c.Highlight.Name != "":
				rec.ClearHighlight(c.Highlight.Name)
			case c.HighlightOff:
				rec.ClearHighlights()
			default:
				rec.Highlight(c.Highlight)
			}
		}
		return nil
	}

	// The recording's pulse (RunOptions.Progress): the total is the
	// script's promise in deterministic mode; realtime rolls on the
	// wall clock with no declared end.
	var total time.Duration
	if opts.Mode == foley.Deterministic {
		total = declaredTotal(t, settings)
	}
	pulse := func(phase ProgressPhase, output string) {
		if opts.Progress == nil {
			return
		}
		opts.Progress(ProgressEvent{
			Phase: phase, Now: rec.Now(), Total: total,
			Frames: rec.Frames(), Output: output,
		})
	}
	pulse(ProgressRecording, "")

	var clipboard string
	for i, cmd := range t.Commands {
		if err := applyCues(i); err != nil {
			return rep, err
		}
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
		pulse(ProgressRecording, "")
	}
	if err := applyCues(len(t.Commands)); err != nil {
		return rep, err
	}

	// The question every animated-TUI tape raises, answered proactively.
	// One restless settle is already proof the app writes on its own —
	// launch paint and answered keystrokes never count — and the
	// keyframe collapse it warns about applies from the first one.
	if opts.Mode == foley.Deterministic && rec.RestlessSettles() >= restlessWarnThreshold {
		warn("the app wrote output on its own %d time(s) (animation or background activity); deterministic mode records settled keyframes only — run with --mode realtime to capture that motion as it happened", rec.RestlessSettles())
	}

	for _, out := range t.Outputs {
		pulse(ProgressDeveloping, out)
		if err := rec.Output(ctx, out); err != nil {
			return rep, fmt.Errorf("tape: Output %s: %w", out, err)
		}
		rep.Outputs = append(rep.Outputs, out)
	}
	return rep, rec.Close()
}

// scaleDur applies PlaybackSpeed to a declared duration: speed 2
// halves it (the video plays twice as fast).
func scaleDur(settings Settings, d time.Duration) time.Duration {
	if settings.PlaybackSpeed == 1.0 || settings.PlaybackSpeed <= 0 {
		return d
	}
	return time.Duration(float64(d) / settings.PlaybackSpeed)
}

// perKeyDur is the scaled per-keystroke duration of a command,
// honoring an explicit @duration even at zero (VHS paste semantics).
func perKeyDur(settings Settings, cmd Command) time.Duration {
	if cmd.SpeedSet {
		return scaleDur(settings, cmd.Speed)
	}
	return scaleDur(settings, settings.TypingSpeed)
}

// declaredTotal sums the virtual time the script PROMISES: typing (one
// perKey per rune, exactly as the driver spends it), presses and
// sleeps, at their scaled speeds. Waits and settles are
// synchronization — unknowable ahead of the run — so the recording can
// legitimately run past this total; it is a floor, not a bound.
func declaredTotal(t *Tape, settings Settings) time.Duration {
	var total time.Duration
	for _, cmd := range t.Commands {
		switch cmd.Kind {
		case KindType:
			total += time.Duration(len([]rune(cmd.Text))) * perKeyDur(settings, cmd)
		case KindPress:
			total += time.Duration(cmd.Count) * perKeyDur(settings, cmd)
		case KindSleep:
			total += scaleDur(settings, cmd.Speed)
		case KindWait, KindHide, KindShow, KindScreenshot, KindCopy, KindPaste, KindScrollUp, KindScrollDown:
			// Paste types at 0ms; the rest spend no declared time.
		}
	}
	return total
}

// effectiveSettings resolves the settings ONE run records with:
// defaults < dress (the tape's cue, or opts.Dress which REPLACES that
// layer) < the tape's explicit Sets — computed on a COPY, so Run never
// mutates the caller's Tape (parse once, run many: light/dark pairs).
// The keys band follows the same layering: the tape's cue turns it on,
// opts.Keys replaces that switch (ADR-016).
func effectiveSettings(t *Tape, opts RunOptions) (Settings, error) {
	settings := t.Settings
	settings.KeysOverlay, settings.KeysSize = t.KeysCue()
	switch opts.Keys {
	case KeysDefault:
	case KeysOff:
		settings.KeysOverlay = false
	case KeysOnSmall:
		settings.KeysOverlay, settings.KeysSize = true, foley.KeysSmall
	case KeysOnMedium:
		settings.KeysOverlay, settings.KeysSize = true, foley.KeysMedium
	case KeysOnLarge:
		settings.KeysOverlay, settings.KeysSize = true, foley.KeysLarge
	}
	ref := t.DressCue()
	if !opts.Dress.IsZero() {
		ref = opts.Dress
	}
	if !ref.IsZero() && !ref.None {
		d, err := ResolveDress(ref)
		if err != nil {
			return settings, err
		}
		explicit := make(map[string]bool, len(t.Explicit))
		for _, n := range t.Explicit {
			explicit[n] = true
		}
		applyDress(&settings, explicit, d)
	}
	// The theme override is LAST and total: dark/light pairs need the
	// same tape in another palette, explicit Set Theme included.
	if !opts.Theme.IsZero() {
		settings.Theme = opts.Theme
	}
	return settings, nil
}

// ParseThemeRef classifies a theme argument the way Set Theme does — a
// curated name or an inline {json} — and validates it NOW, so a CLI
// typo dies before anything records.
func ParseThemeRef(arg string) (ThemeRef, error) {
	ref := ThemeRef{Name: arg}
	if strings.HasPrefix(strings.TrimSpace(arg), "{") {
		ref = ThemeRef{JSON: arg}
	}
	if _, err := resolveTheme(ref); err != nil {
		return ThemeRef{}, err
	}
	return ref, nil
}

// fontFileFor maps the effective FontFamily to the recorder's FontFile:
// the PATH form (ADR-015) loads that file.
func fontFileFor(family string) string {
	if isFontPath(family) {
		return family
	}
	return ""
}

// fontFamilyFor maps the effective FontFamily to the recorder's
// catalog lookup: the NAME form resolves against the pinned catalog
// (an unknown name is a loud assembly warning, never a system font).
func fontFamilyFor(family string) string {
	if family == "" || isFontPath(family) {
		return ""
	}
	return family
}

// warnStaged emits the ADR-008 tier-2/3 warnings — only for settings the
// tape explicitly asked for.
func warnStaged(t *Tape, mode foley.Mode, warn func(string, ...any)) {
	for _, name := range t.Explicit {
		switch name {
		// FontFamily no longer warns here: the PATH form loads that
		// file and the NAME form resolves against the pinned catalog —
		// an unknown name warns at assembly, catalog listed (ADR-015).
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
