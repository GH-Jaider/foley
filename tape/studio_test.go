package tape

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestStudioSetLifecycle pins the set's shape: the XDG skeleton apps
// expect exists at an EXACT 0700 even under a hardened umask (mkdir
// modes are umask-filtered; the chmod pin is what holds), the stage IS
// home — a path-printing prompt (\w, %~) renders `~`, so the camera
// never sees the set's unique path — and strike leaves nothing.
func TestStudioSetLifecycle(t *testing.T) {
	old := syscall.Umask(0o177)
	defer syscall.Umask(old)
	s, err := buildStudio()
	if err != nil {
		t.Fatal(err)
	}
	if s.stage() != s.home() {
		t.Fatalf("stage %q is not home %q — prompts printing the path would leak the set's unique path", s.stage(), s.home())
	}
	for _, d := range []string{
		s.root,
		s.home(),
		filepath.Join(s.home(), ".config"),
		filepath.Join(s.home(), ".cache"),
		filepath.Join(s.home(), ".local", "share"),
		filepath.Join(s.home(), ".local", "state"),
		s.tmp(),
		s.run(),
	} {
		fi, err := os.Stat(d)
		if err != nil || !fi.IsDir() {
			t.Errorf("set dir %s: %v", d, err)
			continue
		}
		if perm := fi.Mode().Perm(); perm != 0o700 {
			t.Errorf("%s perm = %o under umask 0177, want the chmod-pinned 0700", d, perm)
		}
	}
	if err := s.strike(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.root); !os.IsNotExist(err) {
		t.Fatalf("strike left the set behind at %s: %v", s.root, err)
	}
}

// TestStudioEnvPointsInside pins the layer: every place-naming default
// points into the set, and the env-level identity is the set's own.
func TestStudioEnvPointsInside(t *testing.T) {
	s, err := buildStudio()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.strike() }()
	got := map[string]string{}
	for _, kv := range s.env() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			t.Fatalf("malformed pair %q", kv)
		}
		got[k] = v
	}
	for _, k := range []string{
		"HOME", "TMPDIR", "TMP", "TEMP", "PWD",
		"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
		"XDG_STATE_HOME", "XDG_RUNTIME_DIR",
	} {
		v, ok := got[k]
		if !ok {
			t.Errorf("%s: not in the studio layer", k)
			continue
		}
		if !strings.HasPrefix(v, s.root+string(filepath.Separator)) {
			t.Errorf("%s=%s points outside the set %s", k, v, s.root)
		}
	}
	for k, want := range map[string]string{"USER": "foley", "LOGNAME": "foley", "HOSTNAME": "studio"} {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
	// The world before the set: blanked, never inherited — a host mail
	// path names the user, and bash voices mail arrivals on camera.
	for _, k := range []string{"OLDPWD", "MAIL", "MAILPATH"} {
		if v, ok := got[k]; !ok || v != "" {
			t.Errorf("%s = %q (present %v), want blanked in the layer", k, v, ok)
		}
	}
}

// TestStudioContradictsDir: the studio builds its own working
// directory, so an explicit Dir dies loudly BEFORE anything records —
// engine-free by design, like every pre-flight refusal.
func TestStudioContradictsDir(t *testing.T) {
	tp, err := Parse("Output d.gif\nType \"hi\"\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Run(context.Background(), tp, RunOptions{Studio: true, Dir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "studio") {
		t.Fatalf("err = %v, want the studio/Dir contradiction", err)
	}
}
