package main

import (
	"strings"
	"testing"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/tape"
)

// TestCLISkill pins the agent door: exit 0, the embedded foley.md
// verbatim — frontmatter first, raw markdown, trailing newline — so
// `foley skill > SKILL.md` is byte-faithful on and off a TTY.
func TestCLISkill(t *testing.T) {
	exit, stdout, stderr := cli([]string{"skill"}, "")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if stdout != foley.Skill {
		t.Fatalf("skill output diverges from the embedded foley.md (%d vs %d bytes)", len(stdout), len(foley.Skill))
	}
	if !strings.HasPrefix(stdout, "---\nname: foley\n") {
		t.Fatalf("skill lacks its frontmatter:\n%.120s", stdout)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Fatal("skill output lacks the trailing newline")
	}
	for _, want := range []string{
		"## The agent loop",
		"foley validate",
		"# foley: studio",
		"## The cues",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("skill lacks %q", want)
		}
	}
}

// TestCLISkillArgs pins the door's edges: extra args are a usage
// error, -h is help.
func TestCLISkillArgs(t *testing.T) {
	if exit, _, stderr := cli([]string{"skill", "extra"}, ""); exit != 2 || !strings.Contains(stderr, "usage: foley skill") {
		t.Fatalf("extra arg: exit=%d stderr=%q", exit, stderr)
	}
	if exit, _, _ := cli([]string{"skill", "-h"}, ""); exit != 0 {
		t.Fatalf("-h: exit=%d", exit)
	}
}

// skillKitchenSink exercises every command and cue form foley.md
// teaches — one tape, every construct. The skill answers to the same
// law as the manual: it can never teach a form the parser rejects.
const skillKitchenSink = `Output check.gif
Output check.txt
Output frames/
# foley: studio
# foley: dress foley
# foley: keys small notation=icons accent=#ff4f45

Require bash
Set Shell bash
Set Width 940
Set Height 520
Set FontSize 15
Set FontFamily "JetBrains Mono"
Set LetterSpacing 0.0
Set LineHeight 1.0
Set TypingSpeed 60ms
Set Theme "Catppuccin Mocha"
Set Padding 12
Set Margin 20
Set MarginFill "#1a1614"
Set WindowBar Colorful
Set WindowBarSize 40
Set BorderRadius 8
Set CursorBlink true
Set Framerate 30
Set PlaybackSpeed 1.0
Set LoopOffset 0%
Set WaitTimeout 30s
Set WaitPattern /ready/

Env PS1 "ACTION > "

Hide
Type@0ms "clear"
Enter
Show
Sleep 500ms
Type "echo hello"
Enter
Wait
Wait@10s /hello/
Wait+Screen /hello/
Wait+Line /ready/
Ctrl+C
Ctrl+Shift+P
Ctrl+Alt+Shift+P
Alt+Enter
Shift+Tab
Enter 2
Backspace 3
Delete 1
Insert 1
Tab 2
Space 2
Escape 1
Up 2
Down 2
Left 2
Right 2
PageUp 1
PageDown 1
ScrollUp 3
ScrollDown 3
Screenshot beat.png
Copy "clipboard text"
Paste
Sleep 300ms
# foley: highlight /hello/ 0 as greet
Sleep 300ms
# foley: highlight 0,0 54x13 as box
Sleep 300ms
# foley: highlight off greet
Sleep 200ms
# foley: highlight off
Sleep 200ms
# foley: zoom 0,0 54x13 700ms
Sleep 400ms
# foley: zoom off 700ms
Sleep 300ms
`

func TestSkillTapeFormsParse(t *testing.T) {
	if _, err := tape.Parse(skillKitchenSink); err != nil {
		t.Fatalf("a form the skill teaches does not parse: %v", err)
	}
}
