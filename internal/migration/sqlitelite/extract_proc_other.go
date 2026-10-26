//go:build !windows

package sqlitelite

import "os/exec"

func hideWindow(*exec.Cmd) {}
