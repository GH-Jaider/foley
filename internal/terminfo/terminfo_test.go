package terminfo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestDirIn pins the materialization contract: all four lookup paths
// exist with the exact pinned bytes, the call is idempotent, and a
// corrupted entry heals on the next call.
func TestDirIn(t *testing.T) {
	base := t.TempDir()
	root, err := dirIn(base)
	if err != nil {
		t.Fatalf("dirIn: %v", err)
	}
	check := func() {
		t.Helper()
		for _, rel := range entryNames {
			//nolint:gosec // fixed table of relative paths under the test root
			got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatalf("entry %s: %v", rel, err)
			}
			if !bytes.Equal(got, entry) {
				t.Fatalf("entry %s: bytes diverge from the pinned blob", rel)
			}
		}
	}
	check()

	again, err := dirIn(base)
	if err != nil || again != root {
		t.Fatalf("idempotency: got (%q, %v), want (%q, nil)", again, err, root)
	}

	hurt := filepath.Join(root, "x", "xterm-ghostty")
	if err := os.WriteFile(hurt, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := dirIn(base); err != nil {
		t.Fatalf("heal: %v", err)
	}
	check()
}
