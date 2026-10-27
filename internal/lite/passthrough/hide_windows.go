//go:build windows

package passthrough

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func hide(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

// runTree starts cmd inside a Job Object that kills every process in it when
// the job is closed, so a timeout (or this shim dying) takes upstream and any
// child it spawned with it instead of leaving them behind.
func runTree(cmd *exec.Cmd) error {
	job, jerr := windows.CreateJobObject(nil, nil)
	if jerr == nil {
		defer windows.CloseHandle(job)
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			jerr = err
		}
	}
	// On timeout: terminate the whole job (falls back to the process alone).
	cmd.Cancel = func() error {
		if jerr == nil {
			if err := windows.TerminateJobObject(job, 1); err == nil {
				return nil
			}
		}
		return cmd.Process.Kill()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if jerr == nil {
		if h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)); err == nil {
			_ = windows.AssignProcessToJobObject(job, h)
			windows.CloseHandle(h)
		}
	}
	return cmd.Wait()
}

// waitStoreFree waits up to budget for the Dolt noms LOCK of every embedded
// database under beadsDir to be free. Upstream blocks on that lock with no
// timeout of its own; a stuck holder (a hung or half-exited bd) then hangs
// every later call. Fails with ErrStoreBusy instead of waiting forever.
func waitStoreFree(beadsDir string, budget time.Duration) error {
	if beadsDir == "" {
		return nil
	}
	locks, _ := filepath.Glob(filepath.Join(beadsDir, "embeddeddolt", "*", ".dolt", "noms", "LOCK"))
	deadline := time.Now().Add(budget)
	for _, p := range locks {
		for {
			busy, err := lockBusy(p)
			if err != nil || !busy {
				break // unreadable or free: let upstream decide
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("%w: %s is locked and stayed locked for %s; find the bd/dolt process still holding this project's store (a hung or half-exited bd.exe) and end it", ErrStoreBusy, p, budget)
			}
			time.Sleep(250 * time.Millisecond)
		}
	}
	return nil
}

// lockBusy tries a non-blocking exclusive byte-range lock on the file.
func lockBusy(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return true, nil
		}
		return false, err
	}
	defer f.Close()
	h := windows.Handle(f.Fd())
	ol := &windows.Overlapped{}
	err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err == windows.ERROR_LOCK_VIOLATION || err == windows.ERROR_IO_PENDING {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	_ = windows.UnlockFileEx(h, 0, 1, 0, ol)
	return false, nil
}
