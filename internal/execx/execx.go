// Package execx is the single seam for external binaries (ADR-013): a
// typed tool table with minimum versions, verified BEFORE first use, and
// context-aware execution whose failures carry the output tail. os/exec
// is forbidden outside this package (depguard); `foley doctor` will read
// the same table.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
)

// Sentinel errors.
var (
	// ErrUnknownTool is returned for a Tool missing from the table.
	ErrUnknownTool = errors.New("execx: unknown tool")
	// ErrToolMissing is returned (wrapped) when the binary is not on PATH.
	ErrToolMissing = errors.New("execx: tool not found on PATH")
	// ErrToolTooOld is returned when the probed version is below the
	// table minimum.
	ErrToolTooOld = errors.New("execx: tool version below minimum")
)

// Tool identifies an external binary from the table.
type Tool string

// The v1 tool table (ADR-013). gifski joins if an ADR brings it in.
const (
	FFmpeg Tool = "ffmpeg"
)

type spec struct {
	versionArgs []string
	// versionRE captures the major version as group 1.
	versionRE *regexp.Regexp
	minMajor  int
}

func specFor(t Tool) (spec, error) {
	switch t {
	case FFmpeg:
		return spec{
			versionArgs: []string{"-version"},
			versionRE:   regexp.MustCompile(`ffmpeg version \D*?(\d+)\.`),
			// The manifest's per-file `option` directive (exact frame
			// timing) is documented since the 6.1 release; older ffmpeg
			// rejects the manifest loudly rather than mistiming it.
			minMajor: 6,
		}, nil
	default:
		return spec{}, fmt.Errorf("%w: %q", ErrUnknownTool, string(t))
	}
}

// Find resolves the tool on PATH and verifies its version against the
// table minimum. A version string the regexp cannot parse (git builds,
// vendor forks) passes: it cannot be older than a released minimum we
// pin, and rejecting it would break legitimate setups.
func Find(ctx context.Context, t Tool) (string, error) {
	sp, err := specFor(t)
	if err != nil {
		return "", err
	}
	path, err := exec.LookPath(string(t))
	if err != nil {
		return "", fmt.Errorf("%w: %s (%v)", ErrToolMissing, t, err)
	}
	out, err := exec.CommandContext(ctx, path, sp.versionArgs...).CombinedOutput() //nolint:gosec // path from LookPath of a table constant
	if err != nil {
		return "", fmt.Errorf("execx: probing %s: %w\n%s", t, err, tail(out))
	}
	m := sp.versionRE.FindSubmatch(out)
	if m == nil {
		return path, nil
	}
	major, err := strconv.Atoi(string(m[1]))
	if err != nil || major < sp.minMajor {
		return "", fmt.Errorf("%w: %s %s < %d", ErrToolTooOld, t, m[1], sp.minMajor)
	}
	return path, nil
}

// Run executes the tool with args, resolving and version-checking it
// first. On failure the error carries the tail of the combined output.
func Run(ctx context.Context, t Tool, args ...string) error {
	path, err := Find(ctx, t)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // path from LookPath of a table constant
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execx: %s: %w\n%s", t, err, tail(out.Bytes()))
	}
	return nil
}

// LookPath resolves an arbitrary program on PATH — the seam behind the
// tape DSL's Require command and shell resolution (this package is the
// only place allowed to touch os/exec).
func LookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%w: %s (%v)", ErrToolMissing, name, err)
	}
	return path, nil
}

// tail keeps error messages honest without flooding them.
func tail(b []byte) []byte {
	const keep = 2048
	if len(b) <= keep {
		return b
	}
	return b[len(b)-keep:]
}

// OpenFile hands a file to the platform's opener — `open` on darwin,
// `xdg-open` elsewhere (the freedesktop standard). No version table:
// any opener is fine. Used by `foley play` as the honest fallback when
// the terminal cannot display graphics itself.
func OpenFile(ctx context.Context, path string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	bin, err := LookPath(name)
	if err != nil {
		return fmt.Errorf("execx: no system opener (%s) on PATH: %w", name, err)
	}
	cmd := exec.CommandContext(ctx, bin, path) //nolint:gosec // bin from LookPath of a platform constant
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execx: %s %s: %w\n%s", name, path, err, tail(out.Bytes()))
	}
	return nil
}
