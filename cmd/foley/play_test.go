package main

import (
	"strings"
	"testing"
)

// TestPlayArgValidation pins the fast failures — all BEFORE any
// terminal probing or recording.
func TestPlayArgValidation(t *testing.T) {
	if exit, _, stderr := cli([]string{"play"}, ""); exit != 2 || !strings.Contains(stderr, "usage: foley play") {
		t.Fatalf("no-args: exit=%d stderr=%q", exit, stderr)
	}
	if exit, _, stderr := cli([]string{"play", "-keys", "gigante", "x.tape"}, ""); exit != 2 || !strings.Contains(stderr, "-keys") {
		t.Fatalf("bad keys: exit=%d stderr=%q", exit, stderr)
	}
	if exit, _, stderr := cli([]string{"play", "-mode", "vivo", "x.tape"}, ""); exit != 2 || !strings.Contains(stderr, "unknown mode") {
		t.Fatalf("bad mode: exit=%d stderr=%q", exit, stderr)
	}
}
