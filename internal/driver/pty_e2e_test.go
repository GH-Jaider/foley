package driver_test

import (
	"context"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/driver"
	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
)

// TestSettleAgainstRealProcess runs the driver over a real pty: the app
// prints a burst and exits, the step's settle absorbs everything (the
// closed chunk channel is the deterministic end-of-output signal — no
// timing at play), and the frame shows the app's output.
func TestSettleAgainstRealProcess(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", "printf 'hola foley'"},
		Size:    ptyx.Winsize{Cols: 40, Rows: 4, XPix: 320, YPix: 64},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	e := fake.New(vtengine.Options{Geometry: vtengine.Geometry{Cols: 40, Rows: 4}})
	r := newRecorder()
	d, err := driver.New(driver.Options{Engine: e, Transport: p, Render: r.render, Sink: r})
	if err != nil {
		t.Fatal(err)
	}

	if err := d.Sleep(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := d.Finish(); err != nil {
		t.Fatal(err)
	}
	want := frameRec{"hola foley", time.Second}
	if len(r.frames) != 1 || r.frames[0] != want {
		t.Fatalf("frames = %+v, want [%+v]", r.frames, want)
	}
}
