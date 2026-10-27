//go:build !windows

package passthrough

import (
	"os/exec"
	"time"
)

func hide(*exec.Cmd) {}

// runTree runs cmd to completion; the context kills it at the limit.
func runTree(cmd *exec.Cmd) error { return cmd.Run() }

// waitStoreFree is a no-op off Windows (upstream's own flock fails fast there).
func waitStoreFree(string, time.Duration) error { return nil }
