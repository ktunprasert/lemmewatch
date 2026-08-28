//go:build !windows

package player

import "os/exec"

func configureProcess(_ *exec.Cmd, _ bool) {}
