package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/GH-Jaider/foley"
)

// runSkill prints the agent-facing manual (foley.md) verbatim: raw
// markdown with skill frontmatter, meant for files and agents — so no
// styling, ever. `foley skill > .claude/skills/foley/SKILL.md` must be
// byte-faithful on and off a TTY.
func runSkill(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("foley skill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "usage: foley skill\n\n"+
			"Prints the agent-facing manual (foley.md): the whole tape\n"+
			"grammar, every `# foley:` cue, the CLI and the authoring\n"+
			"loop, as one loadable skill file. Point an AI agent at it,\n"+
			"or install it as a skill:\n\n"+
			"  foley skill > .claude/skills/foley/SKILL.md\n")
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
	_, _ = io.WriteString(stdout, foley.Skill)
	return 0
}
