package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // test fixture paths come from t.TempDir
		t.Fatal(err)
	}
}

// cli drives run() like the process would, with an empty stdin unless
// the test provides one.
func cli(args []string, stdin string) (exit int, stdout, stderr string) {
	var out, errb bytes.Buffer
	exit = run(args, strings.NewReader(stdin), &out, &errb)
	return exit, out.String(), errb.String()
}

func TestCLIArgErrors(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		exit   int
		stderr string
	}{
		{"no_args", nil, 2, "usage: foley"},
		{"bad_mode", []string{"-mode", "warp", "x.tape"}, 2, "unknown mode"},
		{"missing_file", []string{filepath.Join(t.TempDir(), "no.tape")}, 1, "no such file"},
		{"bad_flag", []string{"-definitely-not-a-flag"}, 2, "flag provided"},
		{"validate_no_args", []string{"validate"}, 2, "usage: foley validate"},
		{"validate_bad_mode", []string{"validate", "-mode", "warp", "x.tape"}, 2, "unknown mode"},
		{"themes_takes_no_args", []string{"themes", "extra"}, 2, "usage: foley themes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exit, _, stderr := cli(c.args, "")
			if exit != c.exit {
				t.Fatalf("exit = %d, want %d (stderr: %s)", exit, c.exit, stderr)
			}
			if !strings.Contains(stderr, c.stderr) {
				t.Fatalf("stderr %q lacks %q", stderr, c.stderr)
			}
		})
	}
}

func TestCLIGrammarErrorsSurface(t *testing.T) {
	tapePath := filepath.Join(t.TempDir(), "bad.tape")
	writeFile(t, tapePath, "Output x.gif\nFoo bar\n")
	exit, _, stderr := cli([]string{tapePath}, "")
	if exit != 1 {
		t.Fatalf("exit = %d (stderr: %s)", exit, stderr)
	}
	// The vendored grammar's line:column shape must reach the user.
	if !strings.Contains(stderr, "│") {
		t.Fatalf("stderr lacks the VHS-style error: %s", stderr)
	}
}

func TestCLIVersion(t *testing.T) {
	exit, stdout, _ := cli([]string{"-version"}, "")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if !strings.HasPrefix(stdout, "foley ") || strings.TrimSpace(stdout) == "foley" {
		t.Fatalf("stdout = %q, want a 'foley <version>' line", stdout)
	}
}

func TestCLIThemesListsCatalog(t *testing.T) {
	exit, stdout, stderr := cli([]string{"themes"}, "")
	if exit != 0 {
		t.Fatalf("exit = %d (stderr: %s)", exit, stderr)
	}
	names := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(names) < 100 {
		t.Fatalf("only %d themes listed", len(names))
	}
	if !sort.StringsAreSorted(names) {
		t.Fatal("themes are not sorted")
	}
	found := false
	for _, n := range names {
		if n == "Dracula" {
			found = true
		}
	}
	if !found {
		t.Fatal("catalog lacks Dracula (vendored themes.json shape changed?)")
	}
}

func TestCLIValidate(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.tape")
	writeFile(t, good, "Output d.gif\nType \"hi\"\nEnter\n")
	warny := filepath.Join(dir, "warny.tape")
	writeFile(t, warny, "Output d.gif\nSet WindowBar Colorful\nCtrl+Enter\n")
	bad := filepath.Join(dir, "bad.tape")
	writeFile(t, bad, "Foo bar\n")

	t.Run("clean_tape_is_quiet", func(t *testing.T) {
		exit, stdout, stderr := cli([]string{"validate", good}, "")
		if exit != 0 || stdout != "" || stderr != "" {
			t.Fatalf("exit=%d stdout=%q stderr=%q, want silent success", exit, stdout, stderr)
		}
	})
	t.Run("warnings_surface_but_pass", func(t *testing.T) {
		exit, _, stderr := cli([]string{"validate", warny}, "")
		if exit != 0 {
			t.Fatalf("exit = %d (warnings must not fail validation): %s", exit, stderr)
		}
		if !strings.Contains(stderr, "WindowBar") || !strings.Contains(stderr, "Ctrl+Enter") {
			t.Fatalf("stderr lacks the expected warnings: %s", stderr)
		}
		if !strings.Contains(stderr, warny+":") {
			t.Fatalf("warnings are not labeled with their file: %s", stderr)
		}
	})
	t.Run("modify_other_keys_silences_chord", func(t *testing.T) {
		exit, _, stderr := cli([]string{"validate", "-modify-other-keys", warny}, "")
		if exit != 0 || strings.Contains(stderr, "Ctrl+Enter") {
			t.Fatalf("exit=%d stderr=%q, want the chord warning gone", exit, stderr)
		}
	})
	t.Run("parse_error_fails_and_names_file", func(t *testing.T) {
		exit, _, stderr := cli([]string{"validate", bad, good}, "")
		if exit != 1 {
			t.Fatalf("exit = %d, want 1", exit)
		}
		if !strings.Contains(stderr, bad+":") {
			t.Fatalf("stderr lacks the failing file: %s", stderr)
		}
	})
	t.Run("stdin_dash", func(t *testing.T) {
		exit, _, stderr := cli([]string{"validate", "-"}, "Output d.gif\nType \"hi\"\n")
		if exit != 0 || stderr != "" {
			t.Fatalf("exit=%d stderr=%q", exit, stderr)
		}
	})
}

// TestCLINoOutputIsAParseError: the vendored grammar requires at least
// one Output, exactly like VHS — the CLI surfaces that loudly instead
// of recording into the void.
func TestCLINoOutputIsAParseError(t *testing.T) {
	tapePath := filepath.Join(t.TempDir(), "silent.tape")
	writeFile(t, tapePath, "Type \"hi\"\n")
	exit, _, stderr := cli([]string{tapePath}, "")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(stderr, "no Output declared") {
		t.Fatalf("stderr lacks the no-Output parse error: %s", stderr)
	}
}
