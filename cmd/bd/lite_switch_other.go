//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach starts the child in its own session with no inherited stdio.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
