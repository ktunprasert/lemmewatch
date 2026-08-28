//go:build windows

package player

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func configureProcess(cmd *exec.Cmd, quiet bool) {
	if quiet {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	}
}
