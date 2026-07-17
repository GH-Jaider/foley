// Package tape parses VHS .tape scripts into a typed AST and executes
// them against a foley Recorder (ADR-008). Parsing is done by the REAL
// VHS grammar, vendored and pinned in internal/vhsgrammar — fidelity
// bug-for-bug; this package is where its stringly commands die: past
// Parse everything is durations, keys, enums and compiled regexps.
package tape

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/GH-Jaider/foley/key"
	"github.com/GH-Jaider/foley/tape/internal/vhsgrammar/lexer"
	"github.com/GH-Jaider/foley/tape/internal/vhsgrammar/parser"
	"github.com/GH-Jaider/foley/tape/internal/vhsgrammar/token"
)

// Kind identifies a timeline action.
type Kind uint8

// Timeline actions.
const (
	KindType Kind = iota
	KindPress
	KindSleep
	KindWait
	KindHide
	KindShow
	KindScreenshot
	KindCopy
	KindPaste
	// KindScrollUp / KindScrollDown are parsed faithfully but staged in
	// execution (mouse-wheel input is pending raster/input work); the
	// executor warns and skips them.
	KindScrollUp
	KindScrollDown
)

// WaitScope selects what a Wait matches against.
type WaitScope uint8

// Wait scopes.
const (
	// WaitLine matches the cursor's current line (VHS default).
	WaitLine WaitScope = iota
	// WaitScreen matches the whole visible screen.
	WaitScreen
)

// Command is one typed timeline action.
type Command struct {
	Kind Kind

	// Text is the payload of Type and Copy, and the path of Screenshot.
	Text string
	// Key, Count and Speed shape a Press (Count repeats; Speed is the
	// per-press duration). Speed also carries Type's per-key override.
	// SpeedSet records whether the tape wrote an explicit @duration:
	// `Type@0ms` means INSTANT (paste semantics), which a zero-means-
	// default sentinel would silently turn into the TypingSpeed.
	Key      key.Key
	Count    int
	Speed    time.Duration
	SpeedSet bool
	// Scope, Pattern and Timeout shape a Wait; nil Pattern and zero
	// Timeout mean the tape's WaitPattern / WaitTimeout.
	Scope   WaitScope
	Pattern *regexp.Regexp
	Timeout time.Duration
}

// Settings is the tape's frontmatter with VHS's own defaults applied.
type Settings struct {
	Shell      string
	FontFamily string
	// FontFiles is a per-style user font family (foley-only: reachable
	// through a dress, no VHS Set exists — ADR-015).
	FontFiles     FontFiles
	FontSize      int
	LetterSpacing float64
	LineHeight    float64
	Framerate     int
	TypingSpeed   time.Duration
	Theme         ThemeRef
	PlaybackSpeed float64
	Width         int
	Height        int
	Padding       int
	LoopOffset    float64 // percent
	MarginFill    string
	Margin        int
	WindowBar     string
	WindowBarSize int
	BorderRadius  int
	// WindowTitle, TitleAlign and WindowBarColor are foley-only (no VHS
	// Set exists); they flow exclusively from a dress. Empty bar color
	// means theme-derived AUTO shading.
	WindowTitle    string
	TitleAlign     string
	WindowBarColor string
	CursorBlink    bool
	WaitTimeout    time.Duration
	WaitPattern    *regexp.Regexp
}

// ThemeRef is a Set Theme argument: a curated theme name or an inline
// JSON literal (exactly VHS's two forms).
type ThemeRef struct {
	Name string
	JSON string
}

// IsZero reports an unset theme (use the default).
func (t ThemeRef) IsZero() bool { return t.Name == "" && t.JSON == "" }

// Tape is a parsed .tape: frontmatter plus the action stream. Source
// commands were already expanded inline by the grammar (upstream
// semantics: sourced Output/Source are dropped).
type Tape struct {
	Settings Settings
	// Explicit lists the setting names the tape set itself, in order —
	// the executor warns about staged/divergent ones only when the tape
	// actually asked for them.
	Explicit []string
	Env      map[string]string
	Requires []string
	Outputs  []string
	Commands []Command
	// LateSets are Set commands that appeared after the first action;
	// they are applied anyway (last wins) but the executor warns: VHS
	// applies settings before recording starts.
	LateSets []string
	// Cues is the tape's `# foley:` cue sheet (ADR-014), in source order.
	Cues []Cue
}

// DressCue returns the tape's dress reference (zero when the tape has no
// dress cue). Parse guarantees at most one.
func (t *Tape) DressCue() DressRef {
	for _, c := range t.Cues {
		if c.Kind == CueDress {
			return c.Dress
		}
	}
	return DressRef{}
}

// vhsDefaults are VHS's own default options (vhs.go/style.go/video.go of
// the pinned release) — a migrated tape must behave as it did there.
func vhsDefaults() Settings {
	return Settings{
		Shell:         "bash",
		FontSize:      22,
		LetterSpacing: 1.0,
		LineHeight:    1.0,
		Framerate:     50,
		TypingSpeed:   50 * time.Millisecond,
		WindowBarSize: 30,
		PlaybackSpeed: 1.0,
		Width:         1200,
		Height:        600,
		Padding:       60,
		CursorBlink:   true,
		WaitTimeout:   15 * time.Second,
		WaitPattern:   regexp.MustCompile(`>$`),
	}
}

// Parse turns tape source into the typed AST. Grammar errors come back
// joined, one per line, in VHS's own "line:col │ message" shape. Source
// commands read files relative to the current working directory, exactly
// like VHS.
func Parse(src string) (*Tape, error) {
	p := parser.New(lexer.New(src))
	cmds := p.Parse()
	if errs := p.Errors(); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.String()
		}
		return nil, fmt.Errorf("tape: %s", strings.Join(msgs, "\n"))
	}

	cues, err := scanCues(src)
	if err != nil {
		return nil, err
	}
	var dressLines []string
	for _, c := range cues {
		if c.Kind == CueDress {
			dressLines = append(dressLines, strconv.Itoa(c.Line))
		}
	}
	if len(dressLines) > 1 {
		return nil, fmt.Errorf("tape: more than one `# foley: dress` cue (lines %s) — a tape has one look",
			strings.Join(dressLines, " and "))
	}

	t := &Tape{
		Settings: vhsDefaults(),
		Env:      map[string]string{},
		Cues:     cues,
	}
	sawAction := false
	for _, c := range cmds {
		acted, err := t.convert(c, sawAction)
		if err != nil {
			return nil, err
		}
		sawAction = sawAction || acted
	}
	if len(t.Outputs) == 0 {
		return nil, errors.New("tape: no Output declared")
	}
	return t, nil
}

// convert folds one grammar command into the tape; it reports whether
// the command was a timeline action (for late-Set tracking).
func (t *Tape) convert(c parser.Command, sawAction bool) (bool, error) {
	switch token.Type(c.Type) {
	case token.SET:
		if sawAction {
			t.LateSets = append(t.LateSets, c.Options)
		}
		t.Explicit = append(t.Explicit, c.Options)
		return false, t.applySet(c.Options, c.Args)
	case token.OUTPUT:
		t.Outputs = append(t.Outputs, c.Args)
		return false, nil
	case token.REQUIRE:
		t.Requires = append(t.Requires, c.Args)
		return false, nil
	case token.ENV:
		t.Env[c.Options] = c.Args
		return false, nil
	case token.TYPE:
		speed, err := optionalDuration(c.Options)
		if err != nil {
			return false, err
		}
		t.Commands = append(t.Commands, Command{Kind: KindType, Text: c.Args, Speed: speed, SpeedSet: c.Options != ""})
	case token.SLEEP:
		d, err := time.ParseDuration(c.Args)
		if err != nil {
			return false, fmt.Errorf("tape: Sleep %q: %w", c.Args, err)
		}
		t.Commands = append(t.Commands, Command{Kind: KindSleep, Speed: d})
	case token.WAIT:
		cmd, err := convertWait(c)
		if err != nil {
			return false, err
		}
		t.Commands = append(t.Commands, cmd)
	case token.HIDE:
		t.Commands = append(t.Commands, Command{Kind: KindHide})
	case token.SHOW:
		t.Commands = append(t.Commands, Command{Kind: KindShow})
	case token.SCREENSHOT:
		t.Commands = append(t.Commands, Command{Kind: KindScreenshot, Text: c.Args})
	case token.COPY:
		t.Commands = append(t.Commands, Command{Kind: KindCopy, Text: c.Args})
	case token.PASTE:
		t.Commands = append(t.Commands, Command{Kind: KindPaste})
	case token.CTRL, token.ALT, token.SHIFT:
		cmd, err := convertModified(token.Type(c.Type), c.Args)
		if err != nil {
			return false, err
		}
		t.Commands = append(t.Commands, cmd)
	case token.BACKSPACE, token.DELETE, token.INSERT, token.ENTER, token.ESCAPE,
		token.TAB, token.SPACE, token.UP, token.DOWN, token.LEFT, token.RIGHT,
		token.PAGE_UP, token.PAGE_DOWN:
		cmd, err := convertKeypress(token.Type(c.Type), c.Options, c.Args)
		if err != nil {
			return false, err
		}
		t.Commands = append(t.Commands, cmd)
	case token.SCROLL_UP, token.SCROLL_DOWN:
		speed, err := optionalDuration(c.Options)
		if err != nil {
			return false, err
		}
		n, err := strconv.Atoi(c.Args)
		if err != nil || n < 1 {
			n = 1
		}
		k := KindScrollUp
		if token.Type(c.Type) == token.SCROLL_DOWN {
			k = KindScrollDown
		}
		t.Commands = append(t.Commands, Command{Kind: k, Count: n, Speed: speed, SpeedSet: c.Options != ""})
	case token.SOURCE:
		// The grammar expands Source inline; a surviving SOURCE command
		// would be an upstream behavior change worth failing loudly on.
		return false, errors.New("tape: unexpected un-expanded Source command")
	default:
		return false, fmt.Errorf("tape: unsupported command %q", c.Type)
	}
	return true, nil
}

func convertWait(c parser.Command) (Command, error) {
	cmd := Command{Kind: KindWait, Scope: WaitLine}
	timeout, err := optionalDuration(c.Options)
	if err != nil {
		return cmd, err
	}
	cmd.Timeout = timeout

	scope, pattern, _ := strings.Cut(c.Args, " ")
	if scope == "Screen" {
		cmd.Scope = WaitScreen
	}
	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return cmd, fmt.Errorf("tape: Wait pattern %q: %w", pattern, err)
		}
		cmd.Pattern = re
	}
	return cmd, nil
}

func convertKeypress(tt token.Type, speed, count string) (Command, error) {
	cmd := Command{Kind: KindPress}
	d, err := optionalDuration(speed)
	if err != nil {
		return cmd, err
	}
	cmd.Speed = d
	cmd.SpeedSet = speed != ""
	n, err := strconv.Atoi(count)
	if err != nil || n < 1 {
		return cmd, fmt.Errorf("tape: repeat count %q for %s", count, tt)
	}
	cmd.Count = n
	k, ok := namedKey(string(tt))
	if !ok {
		return cmd, fmt.Errorf("tape: no key mapping for %s", tt)
	}
	cmd.Key = k
	return cmd, nil
}

// convertModified maps Ctrl/Alt/Shift chords: the grammar hands the
// modifier chain and final key as space-separated literals.
func convertModified(tt token.Type, args string) (Command, error) {
	cmd := Command{Kind: KindPress, Count: 1}
	var mods key.Mod
	switch tt { //nolint:exhaustive // only the three modifier commands reach here
	case token.CTRL:
		mods = key.ModCtrl
	case token.ALT:
		mods = key.ModAlt
	case token.SHIFT:
		mods = key.ModShift
	}
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return cmd, fmt.Errorf("tape: empty %s chord", tt)
	}
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "Ctrl":
			mods |= key.ModCtrl
		case "Alt":
			mods |= key.ModAlt
		case "Shift":
			mods |= key.ModShift
		default:
			return cmd, fmt.Errorf("tape: unknown modifier %q in %s chord", p, tt)
		}
	}
	k, err := literalKey(parts[len(parts)-1], mods)
	if err != nil {
		return cmd, err
	}
	cmd.Key = k.With(mods)
	return cmd, nil
}

// literalKey maps a chord's final element: a single character or a named
// key literal as VHS writes them. Letters normalize to PHYSICAL keys:
// Ctrl/Alt chords use the lowercase base rune (Ctrl+Shift+C is the c key
// with both modifiers — the encoder wants the key, the mods carry the
// shift), while a Shift-only chord is text (Shift+a types "A").
func literalKey(lit string, mods key.Mod) (key.Key, error) {
	if k, ok := namedKey(strings.ToUpper(lit)); ok {
		return k, nil
	}
	r := []rune(lit)
	if len(r) != 1 {
		return key.Key{}, fmt.Errorf("tape: cannot map key %q", lit)
	}
	switch {
	case mods&(key.ModCtrl|key.ModAlt) != 0:
		return key.RuneKey(unicode.ToLower(r[0])), nil
	case mods&key.ModShift != 0:
		return key.RuneKey(unicode.ToUpper(r[0])), nil
	default:
		return key.RuneKey(r[0]), nil
	}
}

// namedKey maps VHS token names (BACKSPACE, PAGE_UP, ...) and their
// literal spellings (Backspace, PageUp, ...) to typed keys.
func namedKey(name string) (key.Key, bool) {
	switch name {
	case "BACKSPACE":
		return key.Named(key.NameBackspace), true
	case "DELETE":
		return key.Named(key.NameDelete), true
	case "INSERT":
		return key.Named(key.NameInsert), true
	case "ENTER":
		return key.Named(key.NameEnter), true
	case "ESCAPE":
		return key.Named(key.NameEscape), true
	case "TAB":
		return key.Named(key.NameTab), true
	case "SPACE":
		return key.Named(key.NameSpace), true
	case "UP":
		return key.Named(key.NameUp), true
	case "DOWN":
		return key.Named(key.NameDown), true
	case "LEFT":
		return key.Named(key.NameLeft), true
	case "RIGHT":
		return key.Named(key.NameRight), true
	case "PAGE_UP", "PAGEUP":
		return key.Named(key.NamePageUp), true
	case "PAGE_DOWN", "PAGEDOWN":
		return key.Named(key.NamePageDown), true
	default:
		return key.Key{}, false
	}
}

func optionalDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("tape: duration %q: %w", s, err)
	}
	return d, nil
}

// applySet folds one Set into the settings; values arrive normalized by
// the grammar (durations carry units, LoopOffset carries '%').
func (t *Tape) applySet(name, value string) error {
	s := &t.Settings
	fail := func(err error) error { return fmt.Errorf("tape: Set %s %q: %w", name, value, err) }
	switch name {
	case "Shell":
		s.Shell = value
	case "FontFamily":
		s.FontFamily = value
	case "FontSize":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fail(err)
		}
		s.FontSize = n
	case "LetterSpacing":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fail(err)
		}
		s.LetterSpacing = f
	case "LineHeight":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fail(err)
		}
		s.LineHeight = f
	case "Framerate":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fail(err)
		}
		s.Framerate = n
	case "TypingSpeed":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fail(err)
		}
		s.TypingSpeed = d
	case "Theme":
		if strings.HasPrefix(strings.TrimSpace(value), "{") {
			s.Theme = ThemeRef{JSON: value}
		} else {
			s.Theme = ThemeRef{Name: value}
		}
	case "PlaybackSpeed":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fail(err)
		}
		s.PlaybackSpeed = f
	case "Width":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fail(err)
		}
		s.Width = n
	case "Height":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fail(err)
		}
		s.Height = n
	case "Padding":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fail(err)
		}
		s.Padding = n
	case "LoopOffset":
		f, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err != nil {
			return fail(err)
		}
		s.LoopOffset = f
	case "MarginFill":
		s.MarginFill = value
	case "Margin":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fail(err)
		}
		s.Margin = n
	case "WindowBar":
		s.WindowBar = value
	case "WindowBarSize":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fail(err)
		}
		s.WindowBarSize = n
	case "BorderRadius":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fail(err)
		}
		s.BorderRadius = n
	case "CursorBlink":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fail(err)
		}
		s.CursorBlink = b
	case "WaitTimeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fail(err)
		}
		s.WaitTimeout = d
	case "WaitPattern":
		re, err := regexp.Compile(value)
		if err != nil {
			return fail(err)
		}
		s.WaitPattern = re
	default:
		return fmt.Errorf("tape: unknown setting %q", name)
	}
	return nil
}
