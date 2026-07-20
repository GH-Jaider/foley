package main

import (
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/tape"
)

// TestCLIManual pins the manual door: exit 0, the man-page sections
// present in order, the VHS reference linked, and no arguments
// accepted.
func TestCLIManual(t *testing.T) {
	exit, stdout, stderr := cli([]string{"manual"}, "")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	last := -1
	for _, sec := range []string{"MANUAL", "SETTINGS", "CUES", "DRESS", "KEYS", "HIGHLIGHT", "ZOOM", "STUDIO", "BUGS", "CREDITS"} {
		i := strings.Index(stdout, "  "+sec+"\n")
		if i == -1 {
			t.Fatalf("manual lacks the %s section:\n%.400s", sec, stdout)
		}
		if i < last {
			t.Fatalf("section %s is out of order", sec)
		}
		last = i
	}
	for _, want := range []string{
		"charmbracelet/vhs#vhs-command-reference",
		"· Output <path>",
		"· Set Shell <string>",
		"· # foley: studio",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("manual lacks %q", want)
		}
	}
}

// TestManualCueFormsParse feeds every cue bullet the manual teaches
// through the real parser (wildcards replaced by concrete values) —
// the manual can never teach a form validate rejects.
func TestManualCueFormsParse(t *testing.T) {
	examples := map[string]string{
		"dress":       "# foley: dress macos",
		"keys":        "# foley: keys small notation=icons accent=#ff4f45 plain",
		"highlight-1": "# foley: highlight /FAIL.*/ 2 as fails",
		"highlight-2": "# foley: highlight 0,1 40x9 as demo",
		// `highlight off <name>` needs that name declared earlier (`as`),
		// so the off form is tested the way a tape would use it.
		"highlight-3": "# foley: highlight /err/ as demo\nSleep 1s\n# foley: highlight off demo",
		"zoom-1":      "# foley: zoom 0,1 40x9 600ms",
		// `zoom off` alone is refused (the camera is already at the
		// full frame), so the off form is taught — and tested — after
		// a push, exactly as a tape would use it.
		"zoom-2": "# foley: zoom 0,1 40x9 600ms\nSleep 1s\n# foley: zoom off 600ms",
		"studio": "# foley: studio",
	}
	if len(examples) != len(tapeCues) {
		t.Fatalf("the manual lists %d cue forms but %d are example-tested — keep them in lockstep", len(tapeCues), len(examples))
	}
	for name, cueLine := range examples {
		src := "Output d.gif\n" + cueLine + "\nType \"hi\"\n"
		if _, err := tape.Parse(src); err != nil {
			t.Fatalf("%s: the manual teaches %q but the parser rejects it: %v", name, cueLine, err)
		}
	}
}

// TestCLIManualArgs pins the door's edges: extra args are a usage
// error, -h is help.
func TestCLIManualArgs(t *testing.T) {
	if exit, _, stderr := cli([]string{"manual", "extra"}, ""); exit != 2 || !strings.Contains(stderr, "usage: foley manual") {
		t.Fatalf("extra arg: exit=%d stderr=%q", exit, stderr)
	}
	if exit, _, stderr := cli([]string{"manual", "-h"}, ""); exit != 0 || !strings.Contains(stderr, "usage: foley manual") {
		t.Fatalf("-h: exit=%d stderr=%q", exit, stderr)
	}
}
