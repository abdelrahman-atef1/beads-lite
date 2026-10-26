//go:build windows

package sqlitestore

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

const driveFixed = 3 // DRIVE_FIXED

// requireLocalFixedDisk refuses a database on a network, removable or RAM
// drive: SQLite WAL needs shared memory and real file locks (research R3).
func requireLocalFixedDisk(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root := filepath.VolumeName(abs)
	if root == "" {
		return fmt.Errorf("cannot determine the drive of %s", abs)
	}
	root += `\`
	p, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	t, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDriveTypeW").Call(uintptr(unsafe.Pointer(p)))
	if t != driveFixed {
		return fmt.Errorf("beads.db must be on a local fixed disk; %s is not (drive type %d)", root, t)
	}
	return nil
}
