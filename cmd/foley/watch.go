package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// -watch re-records on save: the edit→see loop of a frontend, for
// terminal demos. Polling, not fsnotify — a Go dependency cannot be
// "opted into" at runtime (it ships in everyone's binary) and at ~4s
// per render a sub-second poll is invisible; polling also follows the
// PATH, so the atomic-rename saves editors actually do (write temp,
// rename over) never orphan the watch the way inode watchers can.

// watchPoll is how often the watcher checks the files for changes.
const watchPoll = 250 * time.Millisecond

// fileSnapshot is what "changed" means: mtime, size or existence — a
// rename-save flickers through not-existing and lands as a new mtime.
type fileSnapshot struct {
	mtime  time.Time
	size   int64
	exists bool
}

func snapshotAll(paths []string) []fileSnapshot {
	out := make([]fileSnapshot, len(paths))
	for i, p := range paths {
		if st, err := os.Stat(p); err == nil { //nolint:gosec // the watched tape/dress paths are the CLI's whole purpose
			out[i] = fileSnapshot{mtime: st.ModTime(), size: st.Size(), exists: true}
		}
	}
	return out
}

func snapshotsEqual(a, b []fileSnapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// waitForChange blocks until the watched files differ from `before`
// AND have held still for one full poll (the editor finished writing).
// Returns false when ctx cancels — the session is over.
func waitForChange(ctx context.Context, poll time.Duration, paths []string, before []fileSnapshot) bool {
	last := before
	for {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(poll):
		}
		now := snapshotAll(paths)
		if !snapshotsEqual(now, before) && snapshotsEqual(now, last) {
			return true
		}
		last = now
	}
}

// watchLoop drives the session — every re-record is a TAKE: record,
// then wait for a save, forever until ctx cancels. record does one
// full recording (printing its own errors — a broken tape keeps the
// watch alive: you fix it and save) and returns the files to watch
// NEXT round, so Source'd tapes and dress files added by an edit join
// the watch immediately. The watched set snapshots BEFORE each
// recording: a save that lands mid-take rolls again instead of being
// lost.
func watchLoop(ctx context.Context, out io.Writer, poll time.Duration, mainPath string, record func() []string) {
	watch := []string{mainPath}
	take := 1
	for {
		before := snapshotAll(watch)
		newWatch := record()
		if ctx.Err() != nil {
			return
		}
		take++
		if !snapshotsEqual(snapshotAll(watch), before) {
			_, _ = fmt.Fprintf(out, "foley: saved mid-take — rolling take %d\n", take)
			watch = newWatch
			continue
		}
		watch = newWatch
		_, _ = fmt.Fprintf(out, "foley: watching %s — save to roll take %d (ctrl-c wraps)\n", strings.Join(watch, ", "), take)
		if !waitForChange(ctx, poll, watch, snapshotAll(watch)) {
			return
		}
		_, _ = fmt.Fprintf(out, "foley: change detected — rolling take %d\n", take)
	}
}
