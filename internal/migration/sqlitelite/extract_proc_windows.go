//go:build windows

package sqlitelite

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps the child from opening a console window (company rule).
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
