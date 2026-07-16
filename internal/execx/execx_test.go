package execx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/internal/execx"
)

// fakeTool drops an executable `ffmpeg` shell script into a fresh PATH.
func fakeTool(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestFindVersionOK(t *testing.T) {
	fakeTool(t, `echo "ffmpeg version 6.1.1-3ubuntu5 Copyright"`)
	if _, err := execx.Find(context.Background(), execx.FFmpeg); err != nil {
		t.Fatal(err)
	}
}

func TestFindVersionTooOld(t *testing.T) {
	fakeTool(t, `echo "ffmpeg version 3.4.2"`)
	_, err := execx.Find(context.Background(), execx.FFmpeg)
	if !errors.Is(err, execx.ErrToolTooOld) {
		t.Fatalf("err = %v, want ErrToolTooOld", err)
	}
}

func TestFindUnparseableVersionPasses(t *testing.T) {
	// git builds ("N-112...-g...") carry no major; they cannot be older
	// than any released minimum.
	fakeTool(t, `echo "ffmpeg version N-112899-g47e214245b"`)
	if _, err := execx.Find(context.Background(), execx.FFmpeg); err != nil {
		t.Fatal(err)
	}
}

func TestFindMissingTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := execx.Find(context.Background(), execx.FFmpeg)
	if !errors.Is(err, execx.ErrToolMissing) {
		t.Fatalf("err = %v, want ErrToolMissing", err)
	}
}

func TestRunFailureCarriesOutputTail(t *testing.T) {
	fakeTool(t, `if [ "$1" = "-version" ]; then echo "ffmpeg version 6.0"; exit 0; fi
echo "boom: bad argument" >&2
exit 1`)
	err := execx.Run(context.Background(), execx.FFmpeg, "--definitely-bad")
	if err == nil || !strings.Contains(err.Error(), "boom: bad argument") {
		t.Fatalf("error must carry the tool output, got: %v", err)
	}
}

func TestUnknownTool(t *testing.T) {
	_, err := execx.Find(context.Background(), execx.Tool("sox"))
	if !errors.Is(err, execx.ErrUnknownTool) {
		t.Fatalf("err = %v, want ErrUnknownTool", err)
	}
}
