package driver

import (
	"context"
	"regexp"
	"time"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/key"
)

// Timeline is the action surface shared by both clocks. The tape
// executor (M8) records against this interface and picks the clock —
// deterministic (*Driver) or wall (*Realtime) — from the tape settings.
type Timeline interface {
	Type(ctx context.Context, s string, perKey time.Duration) error
	Press(ctx context.Context, k key.Key, dur time.Duration) error
	Sleep(ctx context.Context, d time.Duration) error
	Wait(ctx context.Context, pred func(*vtengine.Frame) bool, timeout time.Duration) error
	WaitText(ctx context.Context, re *regexp.Regexp, timeout time.Duration) error
	Hide() error
	Show() error
	// Scroll shifts the viewport through the scrollback (negative is
	// up) — a view change on the render side; the application never
	// sees it. The scrolled state lands on the timeline like any other
	// visible change: the next advance renders it.
	Scroll(delta int) error
	Screenshot(name string) error
	// ScreenText returns the current visible screen text (the surface
	// waits match against) — the tape DSL's .txt output and debugging.
	ScreenText() (string, error)
	Finish() error
	Now() time.Duration
	// RestlessSettles reports settles where the app wrote with no input
	// to answer (always zero on the wall clock, which collapses nothing)
	// — the tape executor turns it into a "this app animates; use
	// realtime mode" hint.
	RestlessSettles() int
}

var (
	_ Timeline = (*Driver)(nil)
	_ Timeline = (*Realtime)(nil)
)
