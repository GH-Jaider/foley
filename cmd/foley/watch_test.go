package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestWaitForChange pins the watcher's senses: a plain write fires, an
// atomic rename-save (what editors actually do) fires, and a canceled
// context ends the wait.
func TestWaitForChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.tape")
	if err := os.WriteFile(path, []byte("Output a.gif\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	poll := 15 * time.Millisecond

	fire := func(mutate func()) bool {
		t.Helper()
		before := snapshotAll([]string{path})
		done := make(chan bool, 1)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		go func() { done <- waitForChange(ctx, poll, []string{path}, before) }()
		time.Sleep(3 * poll)
		mutate()
		select {
		case v := <-done:
			return v
		case <-time.After(4 * time.Second):
			t.Fatal("waitForChange never fired")
			return false
		}
	}

	if !fire(func() {
		if err := os.WriteFile(path, []byte("Output b.gif\nSleep 1s\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}) {
		t.Fatal("plain write did not fire")
	}

	if !fire(func() {
		tmp := filepath.Join(dir, ".demo.tape.tmp")
		if err := os.WriteFile(tmp, []byte("Output c.gif\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, path); err != nil {
			t.Fatal(err)
		}
	}) {
		t.Fatal("rename-save did not fire")
	}

	// Cancel ends the session.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- waitForChange(ctx, poll, []string{path}, snapshotAll([]string{path})) }()
	cancel()
	select {
	case v := <-done:
		if v {
			t.Fatal("canceled wait reported a change")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled wait never returned")
	}
}

// TestWatchLoopRerecords pins the session: record → save → record
// again, and a save landing MID-recording re-records immediately
// instead of being lost.
func TestWatchLoopRerecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.tape")
	if err := os.WriteFile(path, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	poll := 15 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var runs atomic.Int32
	var out strings.Builder
	record := func() []string {
		n := runs.Add(1)
		if n == 1 {
			// A save lands while the first recording renders.
			if err := os.WriteFile(path, []byte("v2 saved mid-render"), 0o600); err != nil {
				t.Error(err)
			}
		}
		if n >= 3 {
			cancel()
		}
		return []string{path}
	}
	done := make(chan struct{})
	go func() { watchLoop(ctx, &out, poll, path, record); close(done) }()

	// Run 2 comes from the mid-render save; run 3 from a normal save.
	deadline := time.After(5 * time.Second)
	for runs.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("mid-render save never re-recorded (runs=%d, out=%q)", runs.Load(), out.String())
		case <-time.After(poll):
		}
	}
	time.Sleep(3 * poll)
	if err := os.WriteFile(path, []byte("v3"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("watch loop never finished (runs=%d, out=%q)", runs.Load(), out.String())
	}
	if got := runs.Load(); got < 3 {
		t.Fatalf("recordings = %d, want >= 3 (initial + mid-render save + normal save)", got)
	}
	if !strings.Contains(out.String(), "mid-recording") {
		t.Fatalf("mid-render path not taken: %q", out.String())
	}
}
