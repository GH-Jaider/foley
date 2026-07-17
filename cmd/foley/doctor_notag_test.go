//go:build !ghosttyvt

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/internal/testassets"
)

// TestCLIDoctorFailsLoudlyWithoutEngine: with REAL fonts but an untagged
// binary (no engine linked), doctor must reach the engine factory, fail
// loudly and exit 1 — never a false "ready". Tagged builds exclude this
// file: there the engine exists and the e2e covers the ready path.
func TestCLIDoctorFailsLoudlyWithoutEngine(t *testing.T) {
	fonts, err := filepath.Abs(filepath.Join("..", "..", "internal", "fontpack", "fonts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(fonts, "JetBrainsMono-Regular.ttf")); statErr != nil {
		testassets.Require(t, statErr, "make fonts")
	}
	exit, stdout, _ := cli([]string{"doctor", "-fonts", fonts}, "")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 (no engine in untagged builds)", exit)
	}
	if !strings.Contains(stdout, "✗ record") || !strings.Contains(stdout, "NOT ready") {
		t.Fatalf("doctor output lacks the loud engine failure: %q", stdout)
	}
}
