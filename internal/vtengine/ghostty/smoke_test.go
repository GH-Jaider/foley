//go:build ghosttyvt

package ghostty

import "testing"

func TestSmokeLinkAndRun(t *testing.T) {
	if err := smoke(); err != nil {
		t.Fatal(err)
	}
}
