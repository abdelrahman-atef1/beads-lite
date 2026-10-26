//go:build !windows

package passthrough

import "os/exec"

func hide(*exec.Cmd) {}
