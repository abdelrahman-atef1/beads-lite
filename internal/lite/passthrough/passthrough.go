// Package passthrough (W12, research R6) sends calls for projects that are
// still on embedded Dolt to upstream bd 1.3.1, because the CGO_ENABLED=0 fork
// cannot open embedded Dolt. The fork's bd is what PATH/BD_BIN points to, so
// every not-yet-migrated project keeps working unchanged.
//
// Wiring hook for T011 (cmd/bd/store_factory*.go / main): call MaybeExec()
// first thing in main, before any store is opened. It returns only when the
// fork should handle the call itself.
package passthrough

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// EnvUpstream names the upstream bd executable (absolute path).
	EnvUpstream = "BD_LITE_UPSTREAM"
	// EnvActive is set on the child so a fork pointed at itself cannot loop.
	EnvActive = "BD_LITE_PASSTHROUGH"
	// DefaultUpstream is the fixed install path used when EnvUpstream is unset
	// (SRE places upstream bd 1.3.1 here).
	DefaultUpstream = `E:\Tools\bd-upstream-1.3.1\bd.exe`
)

// localCommands always run in the fork, even on a Dolt project: they manage
// the switch itself or need no store.
var localCommands = map[string]bool{"lite-switch": true, "version": true, "help": true, "migrate-to-sqlite": true} // lite: migrate-to-sqlite reads the Dolt project itself

// ExitFunc is replaced in tests.
var ExitFunc = os.Exit

// MaybeExec runs upstream and exits with its code when the project in the
// current directory is embedded Dolt. On any doubt (no project found, a
// beads.db present, unreadable metadata) it returns and the fork proceeds.
func MaybeExec() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	args := os.Args[1:]
	if os.Getenv(EnvActive) != "" || LocalCommand(args) {
		return
	}
	dir, ok := FindBeadsDir(cwd, os.Getenv("BEADS_DIR"))
	if !ok || !IsEmbeddedDolt(dir) {
		return
	}
	code, err := Run(UpstreamPath(), args, os.Environ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "bd: this project is still on the Dolt store and needs upstream bd 1.3.1: %v\n(set %s to its absolute path)\n", err, EnvUpstream)
		ExitFunc(1)
		return
	}
	ExitFunc(code)
}

// LocalCommand reports whether the first non-flag argument is a fork-only command.
func LocalCommand(args []string) bool {
	for _, a := range args {
		if a == "--version" || a == "-v" || a == "--help" || a == "-h" {
			return true
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return localCommands[a]
	}
	return len(args) == 0
}

// UpstreamPath returns BD_LITE_UPSTREAM, else the fixed install path.
func UpstreamPath() string {
	if p := os.Getenv(EnvUpstream); p != "" {
		return p
	}
	return DefaultUpstream
}

// FindBeadsDir resolves the project's .beads folder: BEADS_DIR first, else the
// nearest .beads walking up from start. A .beads/redirect file is followed once.
func FindBeadsDir(start, envDir string) (string, bool) {
	var dir string
	if envDir != "" {
		dir = envDir
	} else {
		for d := start; ; {
			c := filepath.Join(d, ".beads")
			if st, err := os.Stat(c); err == nil && st.IsDir() {
				dir = c
				break
			}
			p := filepath.Dir(d)
			if p == d {
				return "", false
			}
			d = p
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "redirect")); err == nil {
		t := strings.TrimSpace(string(b))
		if t != "" {
			if !filepath.IsAbs(t) {
				t = filepath.Join(filepath.Dir(dir), t)
			}
			dir = filepath.Clean(t)
		}
	}
	st, err := os.Stat(dir)
	return dir, err == nil && st.IsDir()
}

// IsEmbeddedDolt is true when metadata.json says backend dolt (or unset), the
// mode is not server, and no beads.db exists. Server mode is left to the fork.
func IsEmbeddedDolt(beadsDir string) bool {
	if _, err := os.Stat(filepath.Join(beadsDir, "beads.db")); err == nil {
		return false
	}
	b, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		// No metadata but an embedded Dolt folder: still a Dolt project.
		st, e := os.Stat(filepath.Join(beadsDir, "embeddeddolt"))
		return e == nil && st.IsDir()
	}
	var m struct {
		Backend  string `json:"backend"`
		DoltMode string `json:"dolt_mode"`
	}
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	if m.Backend != "" && m.Backend != "dolt" {
		return false
	}
	return m.DoltMode != "server"
}

// Run starts upstream with the same args, env and stdio and returns its exit
// code. upstream must be an absolute path: never resolved by name, so a
// look-in-the-working-folder-first lookup cannot pick up a planted bd.exe.
func Run(upstream string, args, env []string) (int, error) {
	if !filepath.IsAbs(upstream) {
		return 0, fmt.Errorf("upstream path %q is not absolute", upstream)
	}
	if st, err := os.Stat(upstream); err != nil || st.IsDir() {
		return 0, fmt.Errorf("upstream %q not found", upstream)
	}
	if self, err := os.Executable(); err == nil && sameFile(self, upstream) {
		return 0, errors.New("upstream path points at this binary")
	}
	cmd := exec.Command(upstream, args...)
	cmd.Env = append(append([]string{}, env...), EnvActive+"=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	hide(cmd)
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

func sameFile(a, b string) bool {
	x, e1 := os.Stat(a)
	y, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(x, y)
}
