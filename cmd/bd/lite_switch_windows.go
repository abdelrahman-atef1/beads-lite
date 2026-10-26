//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detach starts the child with no console window, outside this console's
// process group, with no inherited stdio.
func detach(c *exec.Cmd) {
	const createNoWindow, detachedProcess = 0x08000000, 0x00000008
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | detachedProcess}
}
