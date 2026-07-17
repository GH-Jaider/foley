package tape

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/GH-Jaider/foley"
)

// themesJSON is VHS's curated theme collection, vendored with the
// grammar (scripts/vendor-vhs.sh) so `Set Theme "Catppuccin Mocha"`
// migrates untouched.
//
//go:embed internal/vhsgrammar/themes.json
var themesJSON []byte

// vhsTheme mirrors VHS's theme JSON shape (themes.go of the pinned
// release): flat hex strings. Selection and CursorAccent have no foley
// equivalent yet (no selection rendering) and are ignored.
type vhsTheme struct {
	Name          string `json:"name"`
	Background    string `json:"background"`
	Foreground    string `json:"foreground"`
	Cursor        string `json:"cursor"`
	Black         string `json:"black"`
	Red           string `json:"red"`
	Green         string `json:"green"`
	Yellow        string `json:"yellow"`
	Blue          string `json:"blue"`
	Magenta       string `json:"magenta"`
	Cyan          string `json:"cyan"`
	White         string `json:"white"`
	BrightBlack   string `json:"brightBlack"`
	BrightRed     string `json:"brightRed"`
	BrightGreen   string `json:"brightGreen"`
	BrightYellow  string `json:"brightYellow"`
	BrightBlue    string `json:"brightBlue"`
	BrightMagenta string `json:"brightMagenta"`
	BrightCyan    string `json:"brightCyan"`
	BrightWhite   string `json:"brightWhite"`
}

// Themes lists the vendored VHS theme names accepted by
// `Set Theme "<name>"`, sorted — the CLI's `foley themes` and anything
// that wants to offer the catalog.
func Themes() ([]string, error) {
	var all []vhsTheme
	if err := json.Unmarshal(themesJSON, &all); err != nil {
		return nil, fmt.Errorf("tape: vendored themes.json: %w", err)
	}
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names, nil
}

// resolveTheme turns a ThemeRef into a foley.Theme: by curated name,
// by inline JSON literal, or the foley default when unset. Errors are
// UNFRAMED — each caller blames its own knob (a tape's Set Theme vs a
// dress's theme field), never a knob the user did not write.
func resolveTheme(ref ThemeRef) (foley.Theme, error) {
	switch {
	case ref.IsZero():
		return foley.DefaultTheme(), nil
	case ref.JSON != "":
		var vt vhsTheme
		if err := json.Unmarshal([]byte(ref.JSON), &vt); err != nil {
			return foley.Theme{}, fmt.Errorf("palette JSON: %w", err)
		}
		return themeFromVHS(vt)
	default:
		var all []vhsTheme
		if err := json.Unmarshal(themesJSON, &all); err != nil {
			return foley.Theme{}, fmt.Errorf("vendored themes.json (build defect): %w", err)
		}
		for _, vt := range all {
			if vt.Name == ref.Name {
				return themeFromVHS(vt)
			}
		}
		return foley.Theme{}, fmt.Errorf("unknown theme %q (`foley themes` lists the curated set)", ref.Name)
	}
}

func themeFromVHS(vt vhsTheme) (foley.Theme, error) {
	// Unset slots inherit the default theme — the xterm failure mode
	// (its palette survives a partial theme). Zeroing them would paint
	// missing colors BLACK, which no real terminal does. Upstream's own
	// struct silently drops unknown keys (all.tape's "purple" included);
	// we match that drop bug-for-bug and diverge only in inheriting sane
	// colors for whatever ends up unset.
	t := foley.DefaultTheme()
	var err error
	set := func(dst *foley.RGB, hexs, field string) {
		if err != nil || hexs == "" {
			return
		}
		var c foley.RGB
		c, err = parseHexColor(hexs)
		if err != nil {
			err = fmt.Errorf("theme %s %s: %w", vt.Name, field, err)
			return
		}
		*dst = c
	}
	set(&t.Foreground, vt.Foreground, "foreground")
	set(&t.Background, vt.Background, "background")
	set(&t.Cursor, vt.Cursor, "cursor")
	ansi := []struct {
		i    int
		hexs string
		name string
	}{
		{0, vt.Black, "black"},
		{1, vt.Red, "red"},
		{2, vt.Green, "green"},
		{3, vt.Yellow, "yellow"},
		{4, vt.Blue, "blue"},
		{5, vt.Magenta, "magenta"},
		{6, vt.Cyan, "cyan"},
		{7, vt.White, "white"},
		{8, vt.BrightBlack, "brightBlack"},
		{9, vt.BrightRed, "brightRed"},
		{10, vt.BrightGreen, "brightGreen"},
		{11, vt.BrightYellow, "brightYellow"},
		{12, vt.BrightBlue, "brightBlue"},
		{13, vt.BrightMagenta, "brightMagenta"},
		{14, vt.BrightCyan, "brightCyan"},
		{15, vt.BrightWhite, "brightWhite"},
	}
	for _, a := range ansi {
		set(&t.ANSI[a.i], a.hexs, a.name)
	}
	return t, err
}

func parseHexColor(s string) (foley.RGB, error) {
	if len(s) != 7 || s[0] != '#' {
		return foley.RGB{}, fmt.Errorf("expected #RRGGBB, got %q", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return foley.RGB{}, fmt.Errorf("%q: %w", s, err)
	}
	return foley.RGB{
		R: uint8(v >> 16), //nolint:gosec // 24-bit value by construction
		G: uint8(v >> 8),  //nolint:gosec
		B: uint8(v),       //nolint:gosec
	}, nil
}
