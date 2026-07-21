package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// The manual is a man page written FOR the terminal, in the register
// of `vhs manual`: sections in caps, · bullets, the keyword accented.
// The grammar facts mirror the pinned token table and the cue facts
// mirror tape/cue.go — a change there lands here in the same commit
// (a test pins that every cue form taught below parses).

// vhsReferenceURL is where the base grammar's full documentation
// lives — usage and examples for every command, maintained by VHS.
const vhsReferenceURL = "https://github.com/charmbracelet/vhs#vhs-command-reference"

// issuesURL is where foley's bugs go.
const issuesURL = "https://github.com/GH-Jaider/Foley/issues"

// manItem is one bullet: an optional plain prefix, the accented
// keyword, and the plain remainder ("Set " + "Shell" + " <string>").
type manItem struct{ pre, key, rest string }

// tapeCommands is every command a tape can carry, per the pinned
// grammar's token table.
//
//nolint:gochecknoglobals // immutable manual content
var tapeCommands = []manItem{
	{"", "Output", " <path>"},
	{"", "Require", " <program>"},
	{"", "Set", " <setting> <value>"},
	{"", "Env", " <key> <value>"},
	{"", "Type", "[@<time>] \"<string>\""},
	{"", "Sleep", " <time>"},
	{"", "Wait", "[+Screen|+Line][@<timeout>] [/<regexp>/]"},
	{"", "Ctrl", "[+Alt][+Shift]+<char>"},
	{"", "Alt", "+<key>"},
	{"", "Shift", "+<key>"},
	{"", "Enter", " [repeat]"},
	{"", "Backspace", " [repeat]"},
	{"", "Delete", " [repeat]"},
	{"", "Insert", " [repeat]"},
	{"", "Tab", " [repeat]"},
	{"", "Space", " [repeat]"},
	{"", "Escape", " [repeat]"},
	{"", "Up", " [repeat]"},
	{"", "Down", " [repeat]"},
	{"", "Left", " [repeat]"},
	{"", "Right", " [repeat]"},
	{"", "Home", ""},
	{"", "End", ""},
	{"", "PageUp", " [repeat]"},
	{"", "PageDown", " [repeat]"},
	{"", "ScrollUp", " [repeat]"},
	{"", "ScrollDown", " [repeat]"},
	{"", "Hide", ""},
	{"", "Show", ""},
	{"", "Screenshot", " <path>.png"},
	{"", "Copy", " \"<string>\""},
	{"", "Paste", ""},
	{"", "Source", " <path>.tape"},
}

// tapeSettings is every Set the pinned grammar accepts.
//
//nolint:gochecknoglobals // immutable manual content
var tapeSettings = []manItem{
	{"Set ", "Shell", " <string>"},
	{"Set ", "FontFamily", " <string>"},
	{"Set ", "FontSize", " <number>"},
	{"Set ", "Width", " <number>"},
	{"Set ", "Height", " <number>"},
	{"Set ", "LetterSpacing", " <float>"},
	{"Set ", "LineHeight", " <float>"},
	{"Set ", "TypingSpeed", " <time>"},
	{"Set ", "Theme", " <name|{json}>"},
	{"Set ", "Padding", " <number>"},
	{"Set ", "Margin", " <number>"},
	{"Set ", "MarginFill", " <#color>"},
	{"Set ", "WindowBar", " <type>"},
	{"Set ", "WindowBarSize", " <number>"},
	{"Set ", "BorderRadius", " <number>"},
	{"Set ", "CursorBlink", " <boolean>"},
	{"Set ", "Framerate", " <number>"},
	{"Set ", "PlaybackSpeed", " <float>"},
	{"Set ", "LoopOffset", " <%>"},
	{"Set ", "WaitTimeout", " <time>"},
	{"Set ", "WaitPattern", " <regexp>"},
}

// tapeCues is the `# foley:` layer, every form. The test feeds each
// example through the real parser.
//
//nolint:gochecknoglobals // immutable manual content
var tapeCues = []manItem{
	{"# foley: ", "dress", " <name | ./file.json | {json} | none>"},
	{"# foley: ", "keys", " [small|medium|large] [notation=keycap|icons] [accent=<ansi|#hex|off>] [plain]"},
	{"# foley: ", "highlight", " /<regexp>/ [<n>] [as <name>]"},
	{"# foley: ", "highlight", " <col>,<row> <w>x<h> [as <name>]"},
	{"# foley: ", "highlight", " off [<name>]"},
	{"# foley: ", "zoom", " <col>,<row> <w>x<h> [<duration>]"},
	{"# foley: ", "zoom", " off [<duration>]"},
	{"# foley: ", "studio", ""},
}

// runManual prints the manual: the whole tape language — commands,
// settings and the `# foley:` cues — as a styled man page, the way
// `vhs manual` reads. Off a TTY it degrades to plain text.
func runManual(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("foley manual", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "usage: foley manual\n\n"+
			"Prints the manual: every tape command and setting (the grammar\n"+
			"is VHS's, pinned) and the `# foley:` cues. Usage and examples\n"+
			"for the base commands: "+vhsReferenceURL+"\n")
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
	_, _ = io.WriteString(stdout, manualDoc(newStyles(stdout)))
	return 0
}

// manualDoc renders the manual against one style kit. Layout mirrors
// `vhs manual`: a 2-space margin, caps section titles, tight lists.
func manualDoc(st styles) string {
	var b strings.Builder
	section := func(title string) {
		b.WriteString("\n  " + st.h2.Render(title) + "\n\n")
	}
	para := func(lines ...string) {
		for _, l := range lines {
			b.WriteString("  " + l + "\n")
		}
		b.WriteString("\n")
	}
	link := func(url string) {
		b.WriteString("  " + st.link.Render(url) + "\n\n")
	}
	list := func(items []manItem) {
		for _, it := range items {
			b.WriteString("  " + st.dim.Render("·") + " " + it.pre + st.accent.Render(it.key) + it.rest + "\n")
		}
		b.WriteString("\n")
	}

	section("MANUAL")
	para(
		"foley renders terminal demos from VHS-style .tape files — without",
		"a terminal window. A tape is a script of commands describing the",
		"recording; the # foley: cues lay post-production on top, and the",
		"footage is never touched.",
	)
	para(
		"The grammar is VHS's own, pinned. Usage and examples for every",
		"command live in VHS's reference:",
	)
	link(vhsReferenceURL)
	para("The following is a list of all possible commands in a tape:")
	list(tapeCommands)

	section("SETTINGS")
	para(
		"Set adjusts the recording: fonts, dimensions, theme, chrome. The",
		"dress cue bundles the window chrome as one named layer.",
	)
	list(tapeSettings)

	section("CUES")
	para(
		"Post-production, written as comments: VHS ignores them, foley",
		"performs them. One cue per line; strict inside the namespace — a",
		"typo is a parse error, never a silent no-op. dress, keys and",
		"studio shape the whole take; highlight and zoom act at their",
		"position in the script.",
	)
	list(tapeCues)

	section("DRESS")
	para(
		"The window's wardrobe as one layer: theme, font, bar, padding,",
		"margins. Built-ins: foley wardrobe; cut your own: foley sew. The",
		"tape's explicit Sets always win.",
	)

	section("KEYS")
	para(
		"The input reel under the window — every keystroke with its exact",
		"timing, recall and chords included. Defaults: medium size, keycap",
		"notation; plain drops the film-strip dressing.",
	)

	section("HIGHLIGHT")
	para(
		"A band of the theme's own Selection color, from that beat of the",
		"script until off. Patterns re-match every frame and <n> picks one",
		"match, 0-based in screen order; cells start at 0,0.",
	)

	section("ZOOM")
	para(
		"The camera: push onto a cell rect, hold, pull back. The duration",
		"is the shot (default 600ms, capped at 10s, no easing knob), and",
		"the push is a 1:1 crop of the 2x master — never an upscale.",
	)

	section("STUDIO")
	para(
		"A closed set for the take: a fresh directory becomes HOME, the",
		"working directory and every temp default, struck when the",
		"recording ends. Your machine stays off camera.",
	)
	para(
		"Set hygiene, not sandboxing: the defaults move, nothing is",
		"forbidden — absolute host paths still work, and kernel identity",
		"(hostname) still shows the host. Hard boundary: the container.",
	)

	section("BUGS")
	b.WriteString("  See GitHub Issues: " + st.link.Render(issuesURL) + "\n\n")

	section("CREDITS")
	para(
		"The tape grammar is VHS's, by Charm — vendored from the pinned",
		"release, MIT. ♥",
	)
	link("https://github.com/charmbracelet/vhs")
	return b.String()
}
