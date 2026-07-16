package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // test fixture paths come from t.TempDir
		t.Fatal(err)
	}
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if got := run(c.args, &out, &errb); got != c.exit {
				t.Fatalf("exit = %d, want %d (stderr: %s)", got, c.exit, errb.String())
			}
			if !strings.Contains(errb.String(), c.stderr) {
				t.Fatalf("stderr %q lacks %q", errb.String(), c.stderr)
			}
		})
	}
}

func TestCLIGrammarErrorsSurface(t *testing.T) {
	tapePath := filepath.Join(t.TempDir(), "bad.tape")
	writeFile(t, tapePath, "Output x.gif\nFoo bar\n")
	var out, errb bytes.Buffer
	if got := run([]string{tapePath}, &out, &errb); got != 1 {
		t.Fatalf("exit = %d (stderr: %s)", got, errb.String())
	}
	// The vendored grammar's line:column shape must reach the user.
	if !strings.Contains(errb.String(), "│") {
		t.Fatalf("stderr lacks the VHS-style error: %s", errb.String())
	}
}
