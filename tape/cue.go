package tape

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// Cue is one `# foley:` post-production line (ADR-014). VHS ignores
// comment lines, so a tape with cues still records everywhere — cues
// only ever ADD. The set of cues in a tape is its cue sheet.
type Cue struct {
	// Line is 1-based in the tape source, for spotting and errors.
	Line int
	Kind CueKind
	// Dress carries the payload when Kind == CueDress.
	Dress DressRef
}

// CueKind identifies a cue type.
type CueKind uint8

// The cue types. dress is the first (ADR-014); zoom, overlay and
// captions extend the same scanner later.
const (
	CueDress CueKind = iota
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
// strict on content — `# Foley : dress warp` is a valid cue, but its
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
	for i, line := range strings.Split(src, "\n") {
		m := cueLineRE.FindStringSubmatch(line)
		if m == nil {
			// M2 of the dress parada: the grammar allows trailing
			// comments, where our marker would silently be plain text.
			if loc := cueAnywhereRE.FindStringIndex(stripQuoted(line)); loc != nil {
				return nil, fmt.Errorf("tape: %d: `# foley:` cues must be on their own line", i+1)
			}
			if sm := sourceRE.FindStringSubmatch(line); sm != nil {
				if err := checkSourcedCues(sm[1], i+1); err != nil {
					return nil, err
				}
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
		default:
			return nil, fmt.Errorf("tape: %d: unknown cue %q (available cues: dress)", i+1, kind)
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
	if d.WindowBar != nil {
		switch *d.WindowBar {
		case "", "Colorful", "ColorfulRight", "Rings", "RingsRight":
		default:
			return fmt.Errorf("windowBar %q unknown (Colorful|ColorfulRight|Rings|RingsRight)", *d.WindowBar)
		}
	}
	for name, v := range map[string]*int{
		"margin": d.Margin, "windowBarSize": d.WindowBarSize,
		"borderRadius": d.BorderRadius, "padding": d.Padding,
	} {
		if v != nil && *v < 0 {
			return fmt.Errorf("%s %d is negative", name, *v)
		}
	}
	if d.MarginFill != nil && strings.HasPrefix(*d.MarginFill, "#") {
		hex := (*d.MarginFill)[1:]
		if len(hex) != 3 && len(hex) != 6 {
			return fmt.Errorf("marginFill %q: hex colors are #RGB or #RRGGBB", *d.MarginFill)
		}
		for _, r := range hex {
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return fmt.Errorf("marginFill %q: invalid hex color", *d.MarginFill)
			}
		}
	}
	return nil
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
