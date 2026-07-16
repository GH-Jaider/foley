package ghostty_test

// The keyprobe fixture lives under testdata/, which `go mod tidy` never
// scans — without this anchor, tidy strips golang.org/x/term from go.mod
// and `make fixtures` breaks on a fresh checkout. Blank-importing it from
// a real (untagged) test file keeps the requirement pinned.
import (
	_ "golang.org/x/term"
)
