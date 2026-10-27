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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

// Guard, when set, is asked before a call is handed to upstream. A non-nil
// error stops the call: it is printed and the process exits with
// GuardExitCode. main wires the beads-lite switch notice and freeze here
// (company-88mo): an embedded-Dolt project is exactly the one that is frozen
// during a switch, and upstream knows nothing about the freeze.
var Guard func(beadsDir string, args []string) error

// GuardExitCode is the exit status of a refused call (switch.md: 3).
const GuardExitCode = 3

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
	if Guard != nil {
		if gerr := Guard(dir, args); gerr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", gerr)
			ExitFunc(GuardExitCode)
			return
		}
	}
	code, err := RunIn(dir, UpstreamPath(), args, os.Environ())
	if errors.Is(err, ErrTimeout) || errors.Is(err, ErrStoreBusy) {
		fmt.Fprintf(os.Stderr, "Error: bd: %v\n", err)
		ExitFunc(code)
		return
	}
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
	if envDir == "" {
		dir = mainTreeBeadsDir(dir)
	}
	st, err := os.Stat(dir)
	return dir, err == nil && st.IsDir()
}

// mainTreeBeadsDir maps a .beads folder inside a linked git worktree to the
// main tree's .beads folder, which is the one bd actually opens (company-y7ft).
// A worktree on an old branch carries its own tracked metadata.json that may
// still say dolt. On any doubt it returns dir unchanged.
func mainTreeBeadsDir(dir string) string {
	root := filepath.Dir(dir)
	b, err := os.ReadFile(filepath.Join(root, ".git")) // a file only in linked worktrees
	if err != nil {
		return dir
	}
	g := strings.TrimSpace(string(b))
	if !strings.HasPrefix(g, "gitdir:") {
		return dir
	}
	gd := filepath.FromSlash(strings.TrimSpace(strings.TrimPrefix(g, "gitdir:")))
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(root, gd)
	}
	// <main>/.git/worktrees/<name>
	gd = filepath.Clean(gd)
	common := filepath.Dir(filepath.Dir(gd))
	if filepath.Base(filepath.Dir(gd)) != "worktrees" || filepath.Base(common) != ".git" {
		return dir
	}
	cand := filepath.Join(filepath.Dir(common), ".beads")
	if st, err := os.Stat(cand); err == nil && st.IsDir() {
		return cand
	}
	return dir
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
func Run(upstream string, args, env []string) (int, error) { return RunIn("", upstream, args, env) }

// RunIn is Run for a known project: beadsDir (may be "") is checked for a
// Dolt store held by another process before upstream is started.
func RunIn(beadsDir, upstream string, args, env []string) (int, error) {
	if !filepath.IsAbs(upstream) {
		return 0, fmt.Errorf("upstream path %q is not absolute", upstream)
	}
	if st, err := os.Stat(upstream); err != nil || st.IsDir() {
		return 0, fmt.Errorf("upstream %q not found", upstream)
	}
	if self, err := os.Executable(); err == nil && sameFile(self, upstream) {
		return 0, errors.New("upstream path points at this binary")
	}
	// A call must fail fast, never hang (company-1bo5 P0, 2026-10-27): upstream
	// waits on Dolt gate locks and the embedded store with no timeout of its
	// own, so the fork bounds it and kills it.
	limit := Timeout(env)
	if err := waitStoreFree(beadsDir, storeWait); err != nil {
		return StoreBusyExitCode, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, upstream, args...)
	cmd.Env = append(append([]string{}, env...), EnvActive+"=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.WaitDelay = 5 * time.Second
	hide(cmd)
	err := runTree(cmd)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return TimeoutExitCode, fmt.Errorf("%w after %s (upstream pid %d and its children killed; set %s to change)", ErrTimeout, limit, cmd.Process.Pid, EnvTimeout)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

const (
	// EnvTimeout bounds one passthrough call (Go duration, e.g. "90s"; 0 or junk = default, capped at MaxTimeout).
	EnvTimeout = "BD_LITE_PASSTHROUGH_TIMEOUT"
	// DefaultTimeout is generous for a Dolt read on a cold, busy machine and far
	// below "10 minutes".
	DefaultTimeout = 120 * time.Second
	// storeWait is how long a call waits for another process to let go of the
	// Dolt store before it gives up (upstream would wait on it forever).
	storeWait = 20 * time.Second
	// StoreBusyExitCode is the exit status of a call refused because the store
	// is held by another process (EX_TEMPFAIL).
	StoreBusyExitCode = 75
	// TimeoutExitCode is the exit status of a call that hit the limit (like timeout(1)).
	TimeoutExitCode = 124
)

// ErrStoreBusy marks a call refused because another process holds the Dolt store.
var ErrStoreBusy = errors.New("dolt store is held by another process")

// ErrTimeout marks a passthrough call killed at its limit.
var ErrTimeout = errors.New("upstream bd timed out")

// MaxTimeout is the longest limit a caller may ask for.
const MaxTimeout = 30 * time.Minute

// Timeout returns the per-call limit from env (an os.Environ()-style slice).
// There is no "no limit": 0, a negative or an unparsable value means
// DefaultTimeout, and anything above MaxTimeout is cut to it. A QA script that
// set =0 once turned the guard off and left calls hanging for an hour.
func Timeout(env []string) time.Duration {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, EnvTimeout+"="); ok && v != "" {
			d, err := time.ParseDuration(v)
			switch {
			case err != nil || d <= 0:
				return DefaultTimeout
			case d > MaxTimeout:
				return MaxTimeout
			}
			return d
		}
	}
	return DefaultTimeout
}

func sameFile(a, b string) bool {
	x, e1 := os.Stat(a)
	y, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(x, y)
}
