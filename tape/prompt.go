package tape

import (
	"regexp"
	"sort"
	"strings"
)

// A custom prompt rides the grammar's own `Env` — foley's job
// is (1) an environment where the tape's PS1 actually WINS, (2) bare
// `Wait` expecting the new prompt, loudly when it cannot.

// mergeEnv layers the recording environment with explicit precedence —
// earlier layers lose to later ones — resolved in a map so the winner
// does not depend on how the libc resolves duplicate keys (glibc's
// getenv returns the FIRST match: without this, a tape's `Env PS1`
// silently LOSES to the shell table on Linux). The result serializes
// SORTED: same tape, same environment, byte for byte.
func mergeEnv(layers ...[]string) []string {
	m := map[string]string{}
	for _, layer := range layers {
		for _, kv := range layer {
			if k, v, ok := strings.Cut(kv, "="); ok {
				m[k] = v
			}
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out
}

// envPairs flattens a tape's Env map into KEY=value pairs. Order does
// not matter — mergeEnv resolves through a map and sorts.
func envPairs(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// promptVar names the environment variable that IS the prompt for the
// shell, or "" for shells that define it with a function (fish, nu,
// xonsh, powershell/pwsh) — those are out of scope, documented, never
// faked.
func promptVar(shellName string) string {
	switch shellName {
	case "bash", "osh":
		return "PS1"
	case "zsh":
		return "PROMPT"
	default:
		return ""
	}
}

// promptWaitPattern resolves what a bare `Wait` should expect. An
// explicit Set WaitPattern always wins; otherwise a customized prompt
// (Env PS1/PROMPT) DERIVES its pattern, or the default stays with a
// LOUD warning — a custom prompt must never be a silent timeout
// factory. Function-prompt shells get the truth instead of fake
// support.
func promptWaitPattern(t *Tape, settings Settings, warn func(string, ...any)) *regexp.Regexp {
	pvar := promptVar(settings.Shell)
	if pvar == "" {
		for _, v := range [...]string{"PS1", "PROMPT"} {
			if _, tried := t.Env[v]; tried {
				warn("Env %s: %s defines its prompt with a function, not a variable — the custom prompt has no effect (custom prompts work in bash and osh via Env PS1, and zsh via Env PROMPT)", v, settings.Shell)
			}
		}
		return settings.WaitPattern
	}
	custom, ok := t.Env[pvar]
	if !ok {
		return settings.WaitPattern
	}
	for _, name := range t.Explicit {
		if name == "WaitPattern" {
			// The tape coordinated prompt and waits itself.
			return settings.WaitPattern
		}
	}
	derived, ok := derivePromptPattern(custom)
	if !ok {
		warn("Env %s %q: could not derive a wait pattern from the dynamic prompt; bare `Wait` still expects %s — pair the prompt with Set WaitPattern", pvar, custom, settings.WaitPattern)
		return settings.WaitPattern
	}
	re, err := regexp.Compile(derived)
	if err != nil {
		// QuoteMeta makes this unreachable; if it ever happens the
		// message must not lie.
		warn("Env %s: derived wait pattern %q does not compile (%v); bare `Wait` keeps %s", pvar, derived, err, settings.WaitPattern)
		return settings.WaitPattern
	}
	warn("Env %s: bare `Wait` now expects the custom prompt (pattern %s)", pvar, derived)
	return re
}

// The prompt's decoration and dynamic tokens, in tape-literal form
// (the string as written in the .tape, e.g. `\[\e[32m\]`) and as raw
// escape bytes:
var (
	// promptColorRE strips what never reaches the SCREEN TEXT: bash's
	// non-printing wrappers, color/attribute escapes (literal-backslash
	// and raw-byte forms), and zsh's style tokens.
	promptColorRE = regexp.MustCompile(`\\\[|\\\]|\\e\[[0-9;]*[a-zA-Z]|\\033\[[0-9;]*[a-zA-Z]|\x1b\[[0-9;]*[a-zA-Z]|%[FK]\{[^}]*\}|%[fkBbSsUu]`)
	// promptDynRE matches tokens whose rendered text is unknowable at
	// parse time: command substitutions whole, bash \X escapes, zsh %X
	// tokens.
	promptDynRE = regexp.MustCompile("\\$\\([^)]*\\)|`[^`]*`|\\\\[a-zA-Z@!#]|%[a-zA-Z~#!?*./]")
)

// derivePromptPattern extracts the static TAIL of a prompt string and
// returns the regex a rendered prompt line ends with. Only the text
// AFTER the last dynamic token is trustworthy — everything before it
// renders as unknowable screen text. An empty tail is underivable:
// the caller warns LOUD.
func derivePromptPattern(ps string) (string, bool) {
	s := ps
	// Multi-line prompts: only the LAST screen line can match a
	// line-anchored wait.
	for _, nl := range [...]string{"\\n", "\n"} {
		if i := strings.LastIndex(s, nl); i >= 0 {
			s = s[i+len(nl):]
		}
	}
	s = promptColorRE.ReplaceAllString(s, "")
	// \$ renders as a plain $ (recordings never run as root) and %%
	// as a literal % — resolve BEFORE the dynamic cut.
	s = strings.ReplaceAll(s, `\$`, "$")
	s = strings.ReplaceAll(s, "%%", "%")
	if locs := promptDynRE.FindAllStringIndex(s, -1); len(locs) > 0 {
		s = s[locs[len(locs)-1][1]:]
	}
	s = strings.TrimRight(s, " \t")
	if s == "" {
		return "", false
	}
	return regexp.QuoteMeta(s) + "$", true
}
