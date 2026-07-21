package main

import (
	"io"

	"github.com/charmbracelet/lipgloss"
)

// styles is the CLI's presentation kit: one brand accent —
// the REC light of the logo — plus the terminal's own status colors,
// bold and faint. Errors stay plain (foley-voice); what dresses up is
// structure and state, never the failure text.
type styles struct {
	accent lipgloss.Style // the REC light: commands, paths, counts
	head   lipgloss.Style // section headings
	h2     lipgloss.Style // manual H2: bold + accent
	ok     lipgloss.Style // ✓ — the terminal's green
	bad    lipgloss.Style // ✗ — the terminal's red
	warn   lipgloss.Style // warnings — the terminal's yellow
	dim    lipgloss.Style // annotations, bullets
	link   lipgloss.Style // URLs — the terminal's green, underlined
}

// newStyles builds the kit against ONE writer: lipgloss probes it, so
// a pipe, CI or a test buffer renders plain byte-identical text
// automatically (TTY detection, NO_COLOR, TERM=dumb). Built per call
// on purpose — no package-level style state (gochecknoglobals).
func newStyles(w io.Writer) styles {
	r := lipgloss.NewRenderer(w)
	// CellArt's REC light (assets/logo) is the one brand color the CLI
	// wears; the light variant keeps contrast on light backgrounds.
	accent := lipgloss.AdaptiveColor{Light: "#C7362D", Dark: "#FF4F45"}
	return styles{
		accent: r.NewStyle().Foreground(accent),
		head:   r.NewStyle().Bold(true),
		h2:     r.NewStyle().Bold(true).Foreground(accent),
		ok:     r.NewStyle().Foreground(lipgloss.Color("2")),
		bad:    r.NewStyle().Foreground(lipgloss.Color("1")),
		warn:   r.NewStyle().Foreground(lipgloss.Color("3")),
		dim:    r.NewStyle().Faint(true),
		link:   r.NewStyle().Foreground(lipgloss.Color("2")).Underline(true),
	}
}
