//go:build windows

package passthrough

import (
	"fmt"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// A noms LOCK held by another handle makes the call fail fast with ErrStoreBusy
// instead of being handed to upstream to wait on forever.
func TestWaitStoreFreeBusy(t *testing.T) {
	beads := t.TempDir()
	lock := filepath.Join(beads, "embeddeddolt", "db", ".dolt", "noms", "LOCK")
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ol := &windows.Overlapped{}
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 0xFFFFFFFF, 0xFFFFFFFF, ol); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := waitStoreFree(beads, 600*time.Millisecond); !errors.Is(err, ErrStoreBusy) {
		t.Fatalf("got %v, want ErrStoreBusy", err)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Fatal("did not wait for the budget")
	}
	if err := windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 0xFFFFFFFF, 0xFFFFFFFF, ol); err != nil {
		t.Fatal(err)
	}
	if err := waitStoreFree(beads, 600*time.Millisecond); err != nil {
		t.Fatalf("free store: %v", err)
	}
	if err := waitStoreFree(t.TempDir(), time.Second); err != nil {
		t.Fatalf("no store: %v", err)
	}
}

// The timeout kills upstream's children too (Job Object), not just upstream.
func TestTimeoutKillsChildren(t *testing.T) {
	me, _ := os.Executable()
	data, _ := os.ReadFile(me)
	self := filepath.Join(t.TempDir(), "standin.exe")
	if err := os.WriteFile(self, data, 0o755); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	code, err := Run(self, nil, []string{"PT_PIDFILE=" + pidFile, EnvTimeout + "=1s"})
	if !errors.Is(err, ErrTimeout) || code != TimeoutExitCode {
		t.Fatalf("code=%d err=%v", code, err)
	}
	b, rerr := os.ReadFile(pidFile)
	if rerr != nil {
		t.Fatalf("stand-in never started its child: %v", rerr)
	}
	var pid uint32
	fmt.Sscan(string(b), &pid)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
		if err != nil {
			return // gone
		}
		ev, _ := windows.WaitForSingleObject(h, 0)
		windows.CloseHandle(h)
		if ev == windows.WAIT_OBJECT_0 {
			return // exited
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("grandchild %d still alive after timeout", pid)
}
