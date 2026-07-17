package tape

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// TestDerivePromptPattern is the audit table (ADR-017): REAL, ugly
// prompts — colors, wrappers, newlines, powerline, substitutions — in
// both tape-literal and raw-byte forms. The derivation's only job is
// the trustworthy static tail; anything dynamic-to-the-end must FAIL
// loudly, never guess.
func TestDerivePromptPattern(t *testing.T) {
	cases := []struct {
		name     string
		ps       string
		want     string
		ok       bool
		rendered string // the prompt LINE as the screen shows it
	}{
		{"plain_arrow", "❯ ", "❯$", true, "❯"},
		{"plain_dollar", "$ ", `\$$`, true, "$"},
		{"venv_paren", "(venv) > ", `\(venv\) >$`, true, "(venv) >"},
		{"bash_colored_arrow", `\[\e[32m\]❯ \[\e[0m\]`, "❯$", true, "❯"},
		{"bash_user_host_cwd_dollar", `\u@\h:\w\$ `, `\$$`, true, "jai@demo:~/src$"},
		{"bash_multiline", `line1\n❯ `, "❯$", true, "❯"},
		{"raw_escape_bytes", "\x1b[1;32m❯\x1b[0m ", "❯$", true, "❯"},
		{"zsh_colored", "%F{blue}❯%f ", "❯$", true, "❯"},
		{"zsh_cwd_percent", "%~ %% ", " %$", true, "~/src %"},
		{"git_substitution_then_arrow", `\w $(git_branch) ❯ `, " ❯$", true, "~/src main ❯"},
		{"backtick_substitution", "`date` ❯ ", " ❯$", true, "Thu Jul 17 ❯"},
		{"powerline_dynamic_tail", `\[\e[0;34m\]\[\e[0;30;44m\] \w \[\e[0;34;49m\]\[\e[0m\] `, "", false, ""},
		{"dynamic_only", `\w`, "", false, ""},
		{"whitespace_only", "   ", "", false, ""},
		{"starship_two_line", `\[\e[1;35m\]\w\[\e[0m\]\n\[\e[1;32m\]❯\[\e[0m\] `, "❯$", true, "❯"},
	}
	for _, c := range cases {
		got, ok := derivePromptPattern(c.ps)
		if ok != c.ok || got != c.want {
			t.Fatalf("%s: derivePromptPattern(%q) = %q %v, want %q %v", c.name, c.ps, got, ok, c.want, c.ok)
		}
		if ok {
			// The derived pattern must match the line the way a bare
			// `Wait` sees it — including the dynamic prefix it skipped.
			if re := regexp.MustCompile(got); !re.MatchString(c.rendered) {
				t.Fatalf("%s: derived %q does not match rendered line %q", c.name, got, c.rendered)
			}
		}
	}
}

// TestMergeEnv pins the environment contract (ADR-017): later layers
// truly win (independent of libc duplicate resolution) and the result
// is sorted — deterministic byte for byte.
func TestMergeEnv(t *testing.T) {
	got := mergeEnv(
		[]string{"PATH=/usr/bin", "PS1=machine", "HOME=/home/x"},
		[]string{"PS1=table> ", "HISTFILE="},
		[]string{"PS1=❯ ", "DEMO=1"},
	)
	want := []string{"DEMO=1", "HISTFILE=", "HOME=/home/x", "PATH=/usr/bin", "PS1=❯ "}
	if len(got) != len(want) {
		t.Fatalf("mergeEnv = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mergeEnv[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestPromptWaitPattern pins the coordination rules: derived pattern
// for env-prompt shells, explicit Set WaitPattern always wins, dynamic
// prompts warn LOUD and keep the default, function-prompt shells get
// the truth.
func TestPromptWaitPattern(t *testing.T) {
	var warns []string
	warn := func(f string, a ...any) { warns = append(warns, fmt.Sprintf(f, a...)) }

	parse := func(src string) *Tape {
		tp, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		return tp
	}

	tp := parse("Output d.gif\nEnv PS1 \"❯ \"\nType \"x\"\n")
	settings := tp.Settings
	re := promptWaitPattern(tp, settings, warn)
	if re.String() != "❯$" {
		t.Fatalf("derived = %q, want ❯$", re.String())
	}

	warns = nil
	tp = parse("Output d.gif\nEnv PS1 \"❯ \"\nSet WaitPattern \"custom$\"\nType \"x\"\n")
	settings = tp.Settings
	if re := promptWaitPattern(tp, settings, warn); re.String() != "custom$" {
		t.Fatalf("explicit Set WaitPattern must win, got %q", re.String())
	}
	if len(warns) != 0 {
		t.Fatalf("explicit coordination must not warn: %v", warns)
	}

	warns = nil
	tp = parse("Output d.gif\nEnv PS1 \"\\w\"\nType \"x\"\n")
	settings = tp.Settings
	if re := promptWaitPattern(tp, settings, warn); re.String() != `>$` {
		t.Fatalf("underivable must keep the default, got %q", re.String())
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "Set WaitPattern") {
		t.Fatalf("underivable must warn with the recipe, got %v", warns)
	}

	warns = nil
	tp = parse("Output d.gif\nSet Shell fish\nEnv PS1 \"❯ \"\nType \"x\"\n")
	settings = tp.Settings
	if re := promptWaitPattern(tp, settings, warn); re.String() != `>$` {
		t.Fatalf("function-prompt shell must keep the default, got %q", re.String())
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "has no effect") {
		t.Fatalf("function-prompt shell must state the truth, got %v", warns)
	}
}
