package tape

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Dress is a named appearance preset that EXPANDS to the VHS chrome
// primitives (ADR-014): the `Set` commands stay the primitives, a dress
// fills them as a base layer, and the tape's explicit Sets always win.
// Pointer fields distinguish "not part of this dress" from an explicit
// zero (a dress may force Padding 0). The JSON shape is public API.
type Dress struct {
	Margin         *int    `json:"margin,omitempty"`
	MarginFill     *string `json:"marginFill,omitempty"`
	WindowBar      *string `json:"windowBar,omitempty"`
	WindowBarSize  *int    `json:"windowBarSize,omitempty"`
	WindowBarColor *string `json:"windowBarColor,omitempty"`
	BorderRadius   *int    `json:"borderRadius,omitempty"`
	Padding        *int    `json:"padding,omitempty"`
	// Foley-only primitives (no VHS Set exists for them): static bar
	// title and its alignment ("center" default, or "left").
	WindowTitle *string `json:"windowTitle,omitempty"`
	TitleAlign  *string `json:"titleAlign,omitempty"`
}

// dressesFS embeds the built-in wardrobe. The presets are foley's own
// editorial "-like" looks (no logos, no trademarks — just proportions
// and bars).
//
//go:embed dresses/*.json
var dressesFS embed.FS

// BuiltinDresses lists the embedded preset names, sorted.
func BuiltinDresses() []string {
	entries, err := dressesFS.ReadDir("dresses")
	if err != nil {
		return nil // embed cannot fail at runtime; belt and suspenders
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names
}

// errUnknownBuiltin distinguishes "no such preset" from a corrupt embed
// (a build defect) — the two must never share an error message.
var errUnknownBuiltin = errors.New("unknown built-in dress")

func builtinDress(name string) (Dress, error) {
	raw, err := dressesFS.ReadFile(path.Join("dresses", name+".json"))
	if err != nil {
		return Dress{}, fmt.Errorf("%w: %q", errUnknownBuiltin, name)
	}
	d, err := parseDressJSON(raw)
	if err != nil {
		// Unreachable in a released binary (tests pin every embed), but
		// if it ever happens the message must not lie.
		return Dress{}, fmt.Errorf("tape: embedded dress %q is corrupt (build defect): %w", name, err)
	}
	return d, nil
}

// ResolveDress turns a reference into its Dress. Names resolve to
// built-ins ONLY (a tape must stay self-contained: your own dresses
// travel as ./file.json or inline). Paths resolve against the CWD, like
// every tape path. Errors are LOUD.
func ResolveDress(ref DressRef) (Dress, error) {
	switch {
	case ref.None || ref.IsZero():
		return Dress{}, nil
	case ref.Name != "":
		d, err := builtinDress(ref.Name)
		if err != nil {
			return Dress{}, fmt.Errorf("tape: dress: %w", err)
		}
		return d, nil
	case ref.JSON != "":
		d, err := parseDressJSON([]byte(ref.JSON))
		if err != nil {
			return Dress{}, fmt.Errorf("tape: dress: %w", err)
		}
		return d, nil
	default:
		raw, err := os.ReadFile(ref.Path) //nolint:gosec // tape-declared path, CWD-relative by doctrine
		if err != nil {
			return Dress{}, fmt.Errorf("tape: dress: %w", err)
		}
		d, err := parseDressJSON(raw)
		if err != nil {
			return Dress{}, fmt.Errorf("tape: dress %s: %w", ref.Path, err)
		}
		return d, nil
	}
}

// applyDress layers a dress under the tape's explicit Sets: a dress
// field lands only where the tape did not `Set` that name itself —
// defaults < dress < explicit Sets (ADR-014). It writes into the given
// COPY of the settings; the parsed Tape itself is never mutated.
func applyDress(s *Settings, explicit map[string]bool, d Dress) {
	if d.Margin != nil && !explicit["Margin"] {
		s.Margin = *d.Margin
	}
	if d.MarginFill != nil && !explicit["MarginFill"] {
		s.MarginFill = *d.MarginFill
	}
	if d.WindowBar != nil && !explicit["WindowBar"] {
		s.WindowBar = *d.WindowBar
	}
	if d.WindowBarSize != nil && !explicit["WindowBarSize"] {
		s.WindowBarSize = *d.WindowBarSize
	}
	if d.WindowBarColor != nil {
		s.WindowBarColor = *d.WindowBarColor
	}
	if d.BorderRadius != nil && !explicit["BorderRadius"] {
		s.BorderRadius = *d.BorderRadius
	}
	if d.Padding != nil && !explicit["Padding"] {
		s.Padding = *d.Padding
	}
	// Foley-only fields have no Set, so no Explicit entry can exist.
	if d.WindowTitle != nil {
		s.WindowTitle = *d.WindowTitle
	}
	if d.TitleAlign != nil {
		s.TitleAlign = *d.TitleAlign
	}
}

// Expansion renders the dress as the `Set` primitives it fills — the
// wardrobe's spotting view (`foley wardrobe <name>`).
func (d Dress) Expansion() []string {
	var out []string
	if d.WindowBar != nil {
		out = append(out, "Set WindowBar "+*d.WindowBar)
	}
	if d.WindowBarSize != nil {
		out = append(out, "Set WindowBarSize "+strconv.Itoa(*d.WindowBarSize))
	}
	if d.WindowBarColor != nil {
		out = append(out, "(foley) WindowBarColor "+strconv.Quote(*d.WindowBarColor))
	}
	if d.BorderRadius != nil {
		out = append(out, "Set BorderRadius "+strconv.Itoa(*d.BorderRadius))
	}
	if d.Margin != nil {
		out = append(out, "Set Margin "+strconv.Itoa(*d.Margin))
	}
	if d.MarginFill != nil {
		// Quoted: unquoted, the grammar's lexer would eat "#..." as a
		// comment — the expansion must round-trip through the very
		// grammar it names.
		out = append(out, "Set MarginFill "+strconv.Quote(*d.MarginFill))
	}
	if d.Padding != nil {
		out = append(out, "Set Padding "+strconv.Itoa(*d.Padding))
	}
	// Foley-only primitives: printed with a marker — there is no Set to
	// paste, they travel only inside a dress.
	if d.WindowTitle != nil {
		out = append(out, "(foley) WindowTitle "+strconv.Quote(*d.WindowTitle))
	}
	if d.TitleAlign != nil {
		out = append(out, "(foley) TitleAlign "+*d.TitleAlign)
	}
	return out
}
