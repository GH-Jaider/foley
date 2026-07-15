// Command foley records scripted terminal demos on a real terminal.
// It is a thin consumer of the public foley API (library first).
package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "foley: CLI under construction — arrives in milestone M8 (docs/ROADMAP.md)")
	os.Exit(1)
}
