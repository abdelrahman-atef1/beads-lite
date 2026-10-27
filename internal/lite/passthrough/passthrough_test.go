package passthrough

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A stuck upstream is killed at the limit and reported, not waited on.
func TestRunTimeout(t *testing.T) {
	me, _ := os.Executable()
	data, err := os.ReadFile(me)
	if err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(t.TempDir(), "standin.exe") // a copy: Run refuses to start itself
	if err := os.WriteFile(self, data, 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	code, err := Run(self, nil, []string{"PT_HANG=1", EnvTimeout + "=500ms"})
	if !errors.Is(err, ErrTimeout) || code != TimeoutExitCode {
		t.Fatalf("code=%d err=%v, want timeout", code, err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("took %s", d)
	}
}

func TestTimeoutParse(t *testing.T) {
	for env, want := range map[string]time.Duration{"": DefaultTimeout, "90s": 90 * time.Second, "0": 0, "junk": DefaultTimeout} {
		if got := Timeout([]string{EnvTimeout + "=" + env}); got != want {
			t.Errorf("%q: got %s want %s", env, got, want)
		}
	}
}

// The test binary doubles as both the fork (PT_MODE=fork) and the stand-in
// upstream bd (started with EnvActive set by Run).
func TestMain(m *testing.M) {
	switch {
	case os.Getenv(EnvActive) != "":
		if os.Getenv("PT_HANG") != "" {
			time.Sleep(time.Minute) // stand-in for upstream stuck on a lock
		}
		fmt.Printf("args=%s active=%s keep=%s\n", strings.Join(os.Args[1:], "|"), os.Getenv(EnvActive), os.Getenv("PT_KEEP"))
		fmt.Fprint(os.Stderr, "err-out")
		os.Exit(7)
	case os.Getenv("PT_MODE") == "fork":
		os.Args = append([]string{"bd"}, strings.Fields(os.Getenv("PT_ARGS"))...)
		if os.Getenv("PT_GUARD") != "" {
			Guard = func(dir string, args []string) error {
				return fmt.Errorf("guard refused %s in %s", args[0], filepath.Base(dir))
			}
		}
		MaybeExec()
		fmt.Print("FORK-CONTINUES")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func mk(t *testing.T, meta string, files ...string) string {
	t.Helper()
	root := t.TempDir()
	b := filepath.Join(root, ".beads")
	os.MkdirAll(filepath.Join(b, "embeddeddolt"), 0o755)
	if meta != "" {
		os.WriteFile(filepath.Join(b, "metadata.json"), []byte(meta), 0o644)
	}
	for _, f := range files {
		os.WriteFile(filepath.Join(b, f), nil, 0o644)
	}
	return root
}

func TestIsEmbeddedDolt(t *testing.T) {
	cases := []struct {
		name, meta string
		files      []string
		want       bool
	}{
		{"dolt", `{"backend":"dolt"}`, nil, true},
		{"unset", `{"database":"dolt"}`, nil, true},
		{"sqlite", `{"backend":"sqlite"}`, nil, false},
		{"dolt+beads.db", `{"backend":"dolt"}`, []string{"beads.db"}, false},
		{"server", `{"backend":"dolt","dolt_mode":"server"}`, nil, false},
		{"bad json", `{`, nil, false},
		{"no metadata", ``, nil, true},
	}
	for _, c := range cases {
		root := mk(t, c.meta, c.files...)
		if got := IsEmbeddedDolt(filepath.Join(root, ".beads")); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestFindBeadsDirWalksUpAndRedirect(t *testing.T) {
	root := mk(t, `{}`)
	sub := filepath.Join(root, "a", "b")
	os.MkdirAll(sub, 0o755)
	got, ok := FindBeadsDir(sub, "")
	if !ok || got != filepath.Join(root, ".beads") {
		t.Fatalf("walk up: %q %v", got, ok)
	}
	other := mk(t, `{}`)
	os.WriteFile(filepath.Join(root, ".beads", "redirect"), []byte(filepath.Join(other, ".beads")+"\n"), 0o644)
	got, _ = FindBeadsDir(sub, "")
	if got != filepath.Join(other, ".beads") {
		t.Fatalf("redirect: %q", got)
	}
}

func TestLocalCommand(t *testing.T) {
	for args, want := range map[string]bool{"lite-switch status": true, "--json lite-switch": true, "list": false, "--version": true, "": true, "show x": false} {
		if got := LocalCommand(strings.Fields(args)); got != want {
			t.Errorf("%q: %v", args, got)
		}
	}
}

func TestRunRejectsBareNameEvenIfInCwd(t *testing.T) {
	// NoDefaultCurrentDirectoryInExePath case: a stand-in in the working
	// folder must never be picked up by bare name.
	self, _ := os.Executable()
	dir := t.TempDir()
	data, _ := os.ReadFile(self)
	os.WriteFile(filepath.Join(dir, "bd-standin.exe"), data, 0o755)
	cwd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(cwd)
	if _, err := Run("bd-standin.exe", nil, nil); err == nil {
		t.Fatal("bare name accepted")
	}
	if _, err := Run(filepath.Join(dir, "missing.exe"), nil, nil); err == nil {
		t.Fatal("missing accepted")
	}
}

func TestRunRefusesSelf(t *testing.T) {
	self, _ := os.Executable()
	if _, err := Run(self, nil, nil); err == nil || !strings.Contains(err.Error(), "this binary") {
		t.Fatalf("err=%v", err)
	}
}

// fork runs the test binary as the fork in dir, with a stand-in upstream copy.
func fork(t *testing.T, dir, args string, extra ...string) (string, string, int) {
	t.Helper()
	self, _ := os.Executable()
	up := filepath.Join(t.TempDir(), "up.exe")
	data, _ := os.ReadFile(self)
	os.WriteFile(up, data, 0o755)
	c := exec.Command(self)
	c.Dir = dir
	c.Env = append(os.Environ(), "PT_MODE=fork", "PT_ARGS="+args, EnvUpstream+"="+up, "PT_KEEP=yes")
	c.Env = append(c.Env, extra...)
	var so, se strings.Builder
	c.Stdout, c.Stderr = &so, &se
	err := c.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return so.String(), se.String(), code
}

func TestMaybeExecPassesThroughOnDolt(t *testing.T) {
	root := mk(t, `{"backend":"dolt"}`)
	out, errOut, code := fork(t, root, "list --json")
	if code != 7 || !strings.Contains(out, "args=list|--json active=1 keep=yes") || errOut != "err-out" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestMaybeExecStaysInForkWhenMigratedOrLocal(t *testing.T) {
	for name, tc := range map[string]struct{ meta, args string }{
		"sqlite":      {`{"backend":"sqlite"}`, "list"},
		"lite-switch": {`{"backend":"dolt"}`, "lite-switch status"},
	} {
		out, _, code := fork(t, mk(t, tc.meta), tc.args)
		if code != 0 || out != "FORK-CONTINUES" {
			t.Errorf("%s: code=%d out=%q", name, code, out)
		}
	}
	// No project at all.
	if out, _, code := fork(t, t.TempDir(), "list"); code != 0 || out != "FORK-CONTINUES" {
		t.Errorf("no project: %d %q", code, out)
	}
}

func TestMaybeExecMissingUpstreamFailsClearly(t *testing.T) {
	root := mk(t, `{"backend":"dolt"}`)
	_, errOut, code := fork(t, root, "list", EnvUpstream+"="+filepath.Join(root, "nope.exe"))
	if code != 1 || !strings.Contains(errOut, "upstream bd 1.3.1") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

// company-88mo: a call for an embedded-Dolt project reaches upstream only when
// the guard (the switch freeze) allows it.
func TestMaybeExecGuardRefusesBeforeUpstream(t *testing.T) {
	root := mk(t, `{"backend":"dolt"}`)
	out, errOut, code := fork(t, root, "create x", "PT_GUARD=1")
	if code != GuardExitCode || strings.Contains(out, "args=") || !strings.Contains(errOut, "guard refused create") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	// No guard: unchanged pass-through.
	if _, _, code := fork(t, root, "create x"); code != 7 {
		t.Fatalf("unguarded code=%d", code)
	}
}

// A linked worktree's tracked metadata.json (dolt) must not win over the main
// tree's sqlite store (company-y7ft).
func TestFindBeadsDirUsesMainTreeFromWorktree(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "main")
	wt := filepath.Join(root, "wt")
	for _, d := range []string{filepath.Join(main, ".beads"), filepath.Join(main, ".git", "worktrees", "wt"), filepath.Join(wt, ".beads")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(main, ".beads", "metadata.json"), []byte(`{"backend":"sqlite"}`), 0o644)
	os.WriteFile(filepath.Join(wt, ".beads", "metadata.json"), []byte(`{"backend":"dolt"}`), 0o644)
	os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+filepath.ToSlash(filepath.Join(main, ".git", "worktrees", "wt"))+"\n"), 0o644)
	dir, ok := FindBeadsDir(wt, "")
	if !ok || dir != filepath.Join(main, ".beads") {
		t.Fatalf("got %q ok=%v", dir, ok)
	}
	if IsEmbeddedDolt(dir) {
		t.Fatal("main tree is sqlite")
	}
	if d, _ := FindBeadsDir(wt, filepath.Join(wt, ".beads")); d != filepath.Join(wt, ".beads") {
		t.Fatalf("BEADS_DIR must win: %q", d)
	}
}
