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

// TestCLISubcommandAfterFlagsGetsAHint: `foley -fonts x doctor` parses
// "doctor" as a tape path; the failure must hint at the real mistake.
func TestCLISubcommandAfterFlagsGetsAHint(t *testing.T) {
	exit, _, stderr := cli([]string{"-fonts", t.TempDir(), "doctor"}, "")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(stderr, "subcommands go before flags") {
		t.Fatalf("stderr lacks the hint: %q", stderr)
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

// TestCLINewScaffold: `foley new` writes a starter tape that its own
// validate accepts, and never overwrites.
func TestCLINewScaffold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.tape")
	exit, stdout, stderr := cli([]string{"new", path}, "")
	if exit != 0 {
		t.Fatalf("exit = %d (stderr: %s)", exit, stderr)
	}
	if !strings.Contains(stdout, "wrote "+path) {
		t.Fatalf("stdout = %q", stdout)
	}
	if exit, _, stderr := cli([]string{"validate", path}, ""); exit != 0 || stderr != "" {
		t.Fatalf("scaffold does not validate cleanly: exit=%d stderr=%q", exit, stderr)
	}
	if exit, _, stderr := cli([]string{"new", path}, ""); exit != 1 || !strings.Contains(stderr, "refusing to overwrite") {
		t.Fatalf("overwrite guard failed: exit=%d stderr=%q", exit, stderr)
	}
	if exit, _, _ := cli([]string{"new"}, ""); exit != 2 {
		t.Fatalf("missing arg: exit = %d, want 2", exit)
	}

	t.Run("help_is_not_a_filename", func(t *testing.T) {
		t.Chdir(t.TempDir())
		for _, flag := range []string{"--help", "-h"} {
			exit, _, stderr := cli([]string{"new", flag}, "")
			if exit != 0 {
				t.Fatalf("new %s: exit = %d, want 0 (help)", flag, exit)
			}
			if !strings.Contains(stderr, "usage: foley new") {
				t.Fatalf("new %s: no usage printed: %q", flag, stderr)
			}
			if _, err := os.Stat(flag); !os.IsNotExist(err) {
				t.Fatalf("new %s CREATED A FILE — a help request must never mutate", flag)
			}
		}
	})
	t.Run("stdin_dash_rejected", func(t *testing.T) {
		exit, _, stderr := cli([]string{"new", "-"}, "")
		if exit != 2 || !strings.Contains(stderr, `"-"`) {
			t.Fatalf("new -: exit=%d stderr=%q, want a clear rejection", exit, stderr)
		}
	})
	t.Run("extension_appended_and_parents_created", func(t *testing.T) {
		dir := t.TempDir()
		bare := filepath.Join(dir, "sub", "demo")
		exit, stdout, stderr := cli([]string{"new", bare}, "")
		if exit != 0 {
			t.Fatalf("exit = %d (stderr: %s)", exit, stderr)
		}
		want := bare + ".tape"
		if !strings.Contains(stdout, "wrote "+want) {
			t.Fatalf("stdout = %q, want mention of %s", stdout, want)
		}
		if exit, _, stderr := cli([]string{"validate", want}, ""); exit != 0 || stderr != "" {
			t.Fatalf("scaffold in subdir does not validate: exit=%d stderr=%q", exit, stderr)
		}
	})
}

// TestCLIDoctorReportsMissingFonts: an empty fonts dir must fail the
// record check loudly (this dies at fontpack, BEFORE the engine — the
// engine-less case is pinned separately in doctor_notag_test.go).
func TestCLIDoctorReportsMissingFonts(t *testing.T) {
	exit, stdout, _ := cli([]string{"doctor", "-fonts", t.TempDir()}, "")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if !strings.Contains(stdout, "✗ record") || !strings.Contains(stdout, "NOT ready") {
		t.Fatalf("doctor output lacks the loud failure: %q", stdout)
	}
	if exit, _, _ := cli([]string{"doctor", "extra"}, ""); exit != 2 {
		t.Fatalf("extra arg: exit = %d, want 2", exit)
	}
}
