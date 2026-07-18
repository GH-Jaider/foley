package tape

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/GH-Jaider/foley"
)

// Cue is one `# foley:` post-production line (ADR-014). VHS ignores
// comment lines, so a tape with cues still records everywhere — cues
// only ever ADD. The set of cues in a tape is its cue sheet.
type Cue struct {
	// Line is 1-based in the tape source, for spotting and errors.
	Line int
	Kind CueKind
	// AfterCommand is how many COMMAND lines precede the cue — its
	// position in the timeline (ADR-018: a highlight acts from between
	// the previous and next command). Counted by the scanner with the
	// inverted keyword list; review on every grammar re-vendor.
	AfterCommand int
	// Dress carries the payload when Kind == CueDress.
	Dress DressRef
	// KeysSize carries the reel size when Kind == CueKeys.
	KeysSize foley.KeysSize
	// Highlight and HighlightOff carry the payload when Kind ==
	// CueHighlight.
	Highlight    foley.HighlightSpec
	HighlightOff bool
}

// CueKind identifies a cue type.
type CueKind uint8

// The cue types: dress (ADR-014), keys (ADR-016) and highlight
// (ADR-018); zoom and captions extend the same scanner later.
const (
	CueDress CueKind = iota
	CueKeys
	CueHighlight
)

// DressRef is a dress argument in one of its four forms — exactly one
// field is set.
type DressRef struct {
	Name string // built-in preset (see BuiltinDresses)
	Path string // a .json file, resolved against the CWD like every tape path
	JSON string // inline JSON object
	None bool   // `dress none`: strip the dress layer
}

// IsZero reports an absent dress reference.
func (d DressRef) IsZero() bool {
	return d.Name == "" && d.Path == "" && d.JSON == "" && !d.None
}

// The marker is generous on form (case, spacing around the colon) and
// strict on content — `# Foley : dress macos` is a valid cue, but its
// BODY must parse or the tape fails loudly.
var cueLineRE = regexp.MustCompile(`(?i)^\s*#\s*foley\s*:\s*(.*)$`)

// cueAnywhereRE spots the marker OUTSIDE the line-start position (a
// trailing comment): the grammar treats it as a plain comment, so
// without this check a typo'd or misplaced cue would vanish silently.
var cueAnywhereRE = regexp.MustCompile(`(?i)#\s*foley\s*:`)

// stripQuoted blanks out "..."/'...'/`...` spans so the trailing-cue
// check cannot fire on string CONTENT (`Type "# foley: x"` is data).
func stripQuoted(line string) string {
	out := []rune(line)
	var quote rune
	for i, r := range out {
		switch {
		case quote == 0 && (r == '"' || r == '\'' || r == '`'):
			quote = r
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			out[i] = ' '
		}
	}
	return string(out)
}

// sourceRE spots top-level Source lines: cues inside sourced tapes are
// unreachable (the grammar expands and drops comments), so a sourced
// file carrying cues must be a loud error, never a silent no-op.
var sourceRE = regexp.MustCompile(`^\s*Source\s+(\S+)`)

// scanCues extracts the cue sheet from raw tape source. Strict inside
// our namespace: a malformed `# foley:` line is a parse error, never a
// silently ignored typo. A plain `# foley` comment (no colon) is not a
// cue.
func scanCues(src string) ([]Cue, error) {
	var cues []Cue
	commands := 0
	for i, line := range strings.Split(src, "\n") {
		m := cueLineRE.FindStringSubmatch(line)
		if m == nil {
			// The grammar allows trailing comments, where our marker
			// would silently be plain text — reject it loudly instead.
			if loc := cueAnywhereRE.FindStringIndex(stripQuoted(line)); loc != nil {
				return nil, fmt.Errorf("tape: %d: `# foley:` cues must be on their own line", i+1)
			}
			if sm := sourceRE.FindStringSubmatch(line); sm != nil {
				if err := checkSourcedCues(sm[1], i+1); err != nil {
					return nil, err
				}
			}
			if isCommandLine(line) {
				commands++
			}
			continue
		}
		text := strings.TrimSpace(m[1])
		if text == "" {
			return nil, fmt.Errorf("tape: %d: empty `# foley:` cue", i+1)
		}
		kind, rest := cutOnSpace(text)
		switch kind {
		case "dress":
			ref, err := ParseDressRef(rest)
			if err != nil {
				return nil, fmt.Errorf("tape: %d: %w", i+1, err)
			}
			cues = append(cues, Cue{Line: i + 1, Kind: CueDress, Dress: ref})
		case "keys":
			size := foley.KeysMedium
			switch rest {
			case "", "medium":
			case "small":
				size = foley.KeysSmall
			case "large":
				size = foley.KeysLarge
			default:
				return nil, fmt.Errorf("tape: %d: keys size %q unknown (small|medium|large)", i+1, rest)
			}
			cues = append(cues, Cue{Line: i + 1, Kind: CueKeys, KeysSize: size})
		case "highlight":
			spec, off, err := parseHighlight(rest)
			if err != nil {
				return nil, fmt.Errorf("tape: %d: %w", i+1, err)
			}
			cues = append(cues, Cue{Line: i + 1, Kind: CueHighlight, AfterCommand: commands, Highlight: spec, HighlightOff: off})
		default:
			return nil, fmt.Errorf("tape: %d: unknown cue %q (available cues: dress, keys, highlight)", i+1, kind)
		}
	}
	return cues, nil
}

// ParseDressRef classifies a dress argument (the same four forms a
// `# foley: dress` cue takes). Built-in names are checked HERE so
// `foley validate` — and a CLI flag — catch a typo before anything
// records.
func ParseDressRef(arg string) (DressRef, error) {
	switch {
	case arg == "":
		return DressRef{}, errors.New("dress: missing argument (a built-in name, ./file.json, an inline {json} or none)")
	case arg == "none":
		return DressRef{None: true}, nil
	case strings.HasPrefix(arg, "{"):
		if _, err := parseDressJSON([]byte(arg)); err != nil {
			return DressRef{}, fmt.Errorf("dress: inline JSON: %w", err)
		}
		return DressRef{JSON: arg}, nil
	case strings.ContainsAny(arg, "/\\") || strings.HasSuffix(arg, ".json"):
		return DressRef{Path: arg}, nil
	default:
		if _, err := builtinDress(arg); err != nil {
			return DressRef{}, fmt.Errorf("dress: %w (list them with `foley wardrobe`; use ./file.json for your own)", err)
		}
		return DressRef{Name: arg}, nil
	}
}

// parseDressJSON decodes a dress with unknown fields REJECTED (a typo'd
// field must never be a silent no-op), trailing garbage rejected
// (`{...} none` is a mistake, not a combination), and values validated
// so `foley validate` catches them before anything records.
func parseDressJSON(raw []byte) (Dress, error) {
	var d Dress
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return Dress{}, err
	}
	if dec.More() {
		return Dress{}, errors.New("trailing data after the dress JSON (the forms do not combine)")
	}
	return d, validateDress(d)
}

// validateDress rejects values the record stage would choke on — the
// error must blame the DRESS, not a Set the tape never wrote.
func validateDress(d Dress) error {
	if d.Theme != nil {
		// Resolve NOW: a typo'd theme name or a malformed palette must
		// die in `foley validate`, never at record time.
		if _, err := resolveTheme(d.Theme.Ref); err != nil {
			return fmt.Errorf("theme: %w", err)
		}
		// Inside a DRESS the palette is strict: Set Theme {json} drops
		// unknown keys bug-for-bug with VHS (ADR'd parity), but our own
		// namespace never swallows a typo'd field silently.
		if d.Theme.Ref.JSON != "" {
			dec := json.NewDecoder(strings.NewReader(d.Theme.Ref.JSON))
			dec.DisallowUnknownFields()
			var vt vhsTheme
			if err := dec.Decode(&vt); err != nil {
				return fmt.Errorf("theme: %w", err)
			}
		}
	}
	if d.FontSize != nil && *d.FontSize <= 0 {
		return fmt.Errorf("fontSize %d: must be positive", *d.FontSize)
	}
	if d.Font != nil {
		if err := validateDressFont(*d.Font); err != nil {
			return err
		}
	}
	if d.WindowBar != nil {
		switch *d.WindowBar {
		case "", "Colorful", "ColorfulRight", "Rings", "RingsRight", "LinuxControls", "GnomeCSD":
		default:
			return fmt.Errorf("windowBar %q unknown (Colorful|ColorfulRight|Rings|RingsRight|LinuxControls|GnomeCSD)", *d.WindowBar)
		}
	}
	if d.TitleAlign != nil {
		switch *d.TitleAlign {
		case "center", "left":
		default:
			return fmt.Errorf("titleAlign %q unknown (center|left)", *d.TitleAlign)
		}
	}
	// A slice, not a map: with two negative fields the blamed one must
	// be the same on every run — even error text is deterministic here.
	for _, f := range []struct {
		name string
		v    *int
	}{
		{"margin", d.Margin},
		{"windowBarSize", d.WindowBarSize},
		{"borderRadius", d.BorderRadius},
		{"padding", d.Padding},
	} {
		if f.v != nil && *f.v < 0 {
			return fmt.Errorf("%s %d is negative", f.name, *f.v)
		}
	}
	if d.WindowBarColor != nil {
		if err := validateHex(*d.WindowBarColor, "windowBarColor"); err != nil {
			return err
		}
	}
	if d.MarginFill != nil && strings.HasPrefix(*d.MarginFill, "#") {
		if err := validateHex(*d.MarginFill, "marginFill"); err != nil {
			return err
		}
	}
	return nil
}

func validateHex(v, field string) error {
	if !strings.HasPrefix(v, "#") {
		return fmt.Errorf("%s %q: hex colors start with #", field, v)
	}
	hex := v[1:]
	if len(hex) != 3 && len(hex) != 6 {
		return fmt.Errorf("%s %q: hex colors are #RGB or #RRGGBB", field, v)
	}
	for _, r := range hex {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return fmt.Errorf("%s %q: invalid hex color", field, v)
		}
	}
	return nil
}

// validateDressFont checks the dress `font` field: the single form is
// a .ttf/.otf path or a pinned catalog family name (never a system
// font — a typo dies HERE, in `foley validate`); the family form needs
// its regular face and path-form files.
func validateDressFont(f DressFont) error {
	if f.Single != "" {
		if !isFontPath(f.Single) && !foley.KnownFontFamily(f.Single) {
			return fmt.Errorf("font %q: not a ./file.ttf path and not in the pinned catalog (%s) — system fonts are non-deterministic, foley refuses them",
				f.Single, strings.Join(foley.FontFamilies(), ", "))
		}
		return nil
	}
	if f.Files.Regular == "" {
		return errors.New("font family: regular is required (metrics derive from it)")
	}
	for _, p := range []string{f.Files.Regular, f.Files.Bold, f.Files.Italic, f.Files.BoldItalic} {
		if p != "" && !isFontPath(p) {
			return fmt.Errorf("font family: %q is not a font file path (.ttf/.otf)", p)
		}
	}
	return nil
}

// isCommandLine reports whether a tape line is a COMMAND (advances the
// timeline) — the positional anchor for cues (ADR-018). INVERTED list:
// anything that is not blank, a comment, or one of the grammar's five
// non-command keywords IS a command. Those five are stable in the
// pinned grammar; review this list on every re-vendor (ADR-008).
func isCommandLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	head, _ := cutOnSpace(trimmed)
	switch head {
	case "Set", "Output", "Env", "Require", "Source":
		return false
	}
	return true
}

// highlightNameRE bounds names to identifier-ish tokens: a typo'd `as`
// clause must fail loudly, not become a strange name.
var highlightNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// parseHighlight classifies a highlight argument (ADR-018):
//
//	off [NAME]
//	/regex/ [first|last|N] [as NAME]
//	COL,ROW WxH [as NAME]
func parseHighlight(arg string) (foley.HighlightSpec, bool, error) {
	switch {
	case arg == "":
		return foley.HighlightSpec{}, false, errors.New("highlight: missing argument (/regex/, COL,ROW WxH, or off)")
	case arg == "off" || strings.HasPrefix(arg, "off "):
		name := strings.TrimSpace(strings.TrimPrefix(arg, "off"))
		if name != "" && !highlightNameRE.MatchString(name) {
			return foley.HighlightSpec{}, false, fmt.Errorf("highlight: off %q: names are letters, digits, _ and -", name)
		}
		return foley.HighlightSpec{Name: name}, true, nil
	case strings.HasPrefix(arg, "/"):
		idx := strings.LastIndex(arg, "/")
		if idx == 0 {
			return foley.HighlightSpec{}, false, fmt.Errorf("highlight: %q: a pattern is wrapped in slashes (/error/)", arg)
		}
		body := arg[1:idx]
		if body == "" {
			return foley.HighlightSpec{}, false, errors.New("highlight: empty pattern")
		}
		re, err := regexp.Compile(body)
		if err != nil {
			return foley.HighlightSpec{}, false, fmt.Errorf("highlight: pattern: %w", err)
		}
		occ, pick, name, err := parseHighlightMods(strings.TrimSpace(arg[idx+1:]), true)
		if err != nil {
			return foley.HighlightSpec{}, false, err
		}
		return foley.HighlightSpec{Pattern: re, Occurrence: occ, Pick: pick, Name: name}, false, nil
	default:
		fields := strings.Fields(arg)
		if len(fields) < 2 {
			return foley.HighlightSpec{}, false, fmt.Errorf("highlight: %q: expected /regex/, COL,ROW WxH (cells), or off", arg)
		}
		var col, row, w, h int
		if n, err := fmt.Sscanf(fields[0]+" "+fields[1], "%d,%d %dx%d", &col, &row, &w, &h); n != 4 || err != nil {
			return foley.HighlightSpec{}, false, fmt.Errorf("highlight: %q: expected /regex/, COL,ROW WxH (cells), or off", arg)
		}
		if col < 0 || row < 0 || w <= 0 || h <= 0 {
			return foley.HighlightSpec{}, false, fmt.Errorf("highlight: rect %q: coordinates start at 0,0 and the size must be positive", arg)
		}
		_, _, name, err := parseHighlightMods(strings.Join(fields[2:], " "), false)
		if err != nil {
			return foley.HighlightSpec{}, false, err
		}
		return foley.HighlightSpec{Col: col, Row: row, W: w, H: h, Rect: true, Name: name}, false, nil
	}
}

// parseHighlightMods parses the optional trailing modifiers: [N]
// (patterns only — the 0-based match index, one standard with the
// rect's cells) and [as NAME].
func parseHighlightMods(rest string, pattern bool) (occ int, pick bool, name string, err error) {
	if rest == "" {
		return 0, false, "", nil
	}
	tokens := strings.Fields(rest)
	i := 0
	if tokens[0] != "as" {
		n := 0
		if _, serr := fmt.Sscanf(tokens[0], "%d", &n); serr != nil {
			return 0, false, "", fmt.Errorf("highlight: %q: expected a 0-based match index or `as NAME`", tokens[0])
		}
		if n < 0 {
			return 0, false, "", fmt.Errorf("highlight: match index %q: indexes start at 0, like the rect's cells", tokens[0])
		}
		occ, pick, i = n, true, 1
	}
	if pick && !pattern {
		return 0, false, "", errors.New("highlight: match indexes need a /pattern/ (a rect has no matches)")
	}
	if i >= len(tokens) {
		return occ, pick, "", nil
	}
	if tokens[i] != "as" || i+1 >= len(tokens) || i+2 < len(tokens) {
		return 0, false, "", fmt.Errorf("highlight: %q: the name clause is `as NAME`", rest)
	}
	name = tokens[i+1]
	if !highlightNameRE.MatchString(name) {
		return 0, false, "", fmt.Errorf("highlight: name %q: letters, digits, _ and -", name)
	}
	return occ, pick, name, nil
}

// isFontPath spots the FontFamily PATH form (ADR-015): a font FILE the
// repo pins (deterministic input), vs a bare NAME resolved against the
// pinned catalog (system fonts are refused).
func isFontPath(s string) bool {
	if strings.ContainsAny(s, "/\\") {
		return true
	}
	l := strings.ToLower(s)
	return strings.HasSuffix(l, ".ttf") || strings.HasSuffix(l, ".otf")
}

// cutOnSpace splits at the first whitespace run (space or tab — a tab
// after the cue kind must not corrupt the kind's name).
func cutOnSpace(s string) (kind, rest string) {
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i:])
}

// checkSourcedCues reads a Source'd tape one level deep and rejects
// cues found there: the grammar's expansion drops comments, so those
// cues would otherwise vanish silently. An unreadable file is left for
// the grammar to report (it owns Source errors).
func checkSourcedCues(path string, line int) error {
	raw, err := os.ReadFile(path) //nolint:gosec // the tape's own Source path, CWD-relative by doctrine
	if err != nil {
		return nil //nolint:nilerr // the vendored grammar reports missing Source files itself
	}
	for _, l := range strings.Split(string(raw), "\n") {
		if cueLineRE.MatchString(l) {
			return fmt.Errorf("tape: %d: Source'd tape %s carries `# foley:` cues — cues must live in the top-level tape", line, path)
		}
	}
	return nil
}
