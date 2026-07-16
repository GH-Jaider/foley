// Package testassets gates tests on provisioned assets (pinned fonts,
// compiled fixtures, committed goldens). Locally a missing asset skips
// the test with a provisioning hint; under CI it FAILS it: the workflow
// provisions every asset, so a skip there would silently drop coverage
// and leave the suite green while testing nothing.
package testassets

import (
	"os"
	"testing"
)

// Require declares that the test needs an asset whose load may have
// failed. A nil err is a no-op. hint tells a local developer how to
// provision the asset (e.g. "make fonts").
func Require(tb testing.TB, err error, hint string) {
	tb.Helper()
	if err == nil {
		return
	}
	if os.Getenv("CI") != "" {
		tb.Fatalf("asset required in CI: %v (%s)", err, hint)
	}
	tb.Skipf("%v — %s", err, hint)
}
