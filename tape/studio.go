package tape

import (
	"fmt"
	"os"
	"path/filepath"
)

// The studio: a closed set for one take. Every tape that
// wants a clean world today hand-rolls the same ritual — `export
// HOME=$(mktemp -d)`, a private TMPDIR, `cd $(mktemp -d)` — and
// forgetting it once puts the author's real home on camera. The studio
// is that ritual as one switch: a fresh directory becomes the recorded
// world's HOME, working directory and every temp/XDG default, and is
// struck (removed) when the take ends.
//
// Set hygiene, not sandboxing: the studio redirects where the DEFAULTS
// point; it forbids nothing. An app given an absolute host path can
// still read it, and identity is pinned at the env level only — an app
// that asks the kernel (zsh's $HOST, a fetch tool's hostname call)
// still sees the host. That boundary belongs to the container, where
// faking it is one flag (`docker run --hostname`).

// studioSet is one take's set. The zero value is not a set — build one
// with buildStudio.
type studioSet struct {
	root string
}

// buildStudio raises the set: HOME with the XDG skeleton apps expect,
// a private tmp, and the runtime dir. Mkdir modes pass through the
// umask (MkdirTemp's included), so every directory is chmod'd to an
// EXACT 0700 — the set is private by contract, and the XDG spec
// requires exactly that of the runtime dir (picky apps verify it).
// Everything lives under one root so strike is one RemoveAll.
func buildStudio() (*studioSet, error) {
	root, err := os.MkdirTemp("", "foley-set-")
	if err != nil {
		return nil, fmt.Errorf("studio: %w", err)
	}
	s := &studioSet{root: root}
	// Parents first: each directory gets its own exact chmod.
	for _, d := range []string{
		root,
		s.home(),
		filepath.Join(s.home(), ".config"),
		filepath.Join(s.home(), ".cache"),
		filepath.Join(s.home(), ".local"),
		filepath.Join(s.home(), ".local", "share"),
		filepath.Join(s.home(), ".local", "state"),
		s.tmp(),
		s.run(),
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			_ = os.RemoveAll(root)
			return nil, fmt.Errorf("studio: %w", err)
		}
		if err := os.Chmod(d, 0o700); err != nil { //nolint:gosec // directories: 0700 IS the floor (x to traverse) and the XDG spec's exact requirement
			_ = os.RemoveAll(root)
			return nil, fmt.Errorf("studio: %w", err)
		}
	}
	return s, nil
}

func (s *studioSet) home() string { return filepath.Join(s.root, "home") }
func (s *studioSet) tmp() string  { return filepath.Join(s.root, "tmp") }
func (s *studioSet) run() string  { return filepath.Join(s.root, "run") }

// stage is the recorded command's working directory — HOME itself, on
// purpose: the set's real path is unique per take, but a prompt that
// prints the path (bash's \w, zsh's %~) renders it as `~`, so what the
// CAMERA sees stays deterministic.
func (s *studioSet) stage() string { return s.home() }

// env is the studio layer of the recording environment: every default
// that names a place on the host now points inside the set, and the
// env-level identity is the set's own — foley@studio, not you@yours.
func (s *studioSet) env() []string {
	return []string{
		"HOME=" + s.home(),
		"TMPDIR=" + s.tmp(),
		"TMP=" + s.tmp(),
		"TEMP=" + s.tmp(),
		"XDG_CONFIG_HOME=" + filepath.Join(s.home(), ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(s.home(), ".cache"),
		"XDG_DATA_HOME=" + filepath.Join(s.home(), ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(s.home(), ".local", "state"),
		"XDG_RUNTIME_DIR=" + s.run(),
		"USER=foley",
		"LOGNAME=foley",
		"HOSTNAME=studio",
		// The world before the set stays out of the env block: PWD is
		// the stage (truthful — the command starts there), OLDPWD is
		// blank (a fresh set has no previous directory), and the mail
		// vars are blanked the HISTFILE way — a host mail path carries
		// the username, and bash would voice "You have mail" ON
		// CAMERA, nondeterministically, if mail landed mid-take.
		"PWD=" + s.home(),
		"OLDPWD=",
		"MAIL=",
		"MAILPATH=",
	}
}

// strike tears the set down. A failure is the caller's to voice — the
// take already succeeded; a leftover set earns a warning naming the
// path, never a lost recording.
func (s *studioSet) strike() error {
	return os.RemoveAll(s.root)
}
