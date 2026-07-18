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
		if st, err := os.Stat(p); err == nil {
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

// watchLoop drives the session: record, then wait for a save, forever
// until ctx cancels. record does one full recording (printing its own
// errors — a broken tape keeps the watch alive: you fix it and save)
// and returns the files to watch NEXT round, so Source'd tapes and
// dress files added by an edit join the watch immediately. The watched
// set snapshots BEFORE each recording: a save that lands mid-render
// re-records right away instead of being lost.
func watchLoop(ctx context.Context, out io.Writer, poll time.Duration, mainPath string, record func() []string) {
	watch := []string{mainPath}
	for {
		before := snapshotAll(watch)
		newWatch := record()
		if ctx.Err() != nil {
			return
		}
		if !snapshotsEqual(snapshotAll(watch), before) {
			_, _ = fmt.Fprintln(out, "foley: change landed mid-recording, re-recording")
			watch = newWatch
			continue
		}
		watch = newWatch
		_, _ = fmt.Fprintf(out, "foley: watching %s — save to re-record (ctrl-c stops)\n", strings.Join(watch, ", "))
		if !waitForChange(ctx, poll, watch, snapshotAll(watch)) {
			return
		}
		_, _ = fmt.Fprintln(out, "foley: change detected, re-recording")
	}
}
