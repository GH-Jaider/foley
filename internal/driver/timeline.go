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
	Screenshot(name string) error
	Finish() error
	Now() time.Duration
}

var (
	_ Timeline = (*Driver)(nil)
	_ Timeline = (*Realtime)(nil)
)
