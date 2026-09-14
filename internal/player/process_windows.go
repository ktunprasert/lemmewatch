//go:build windows

package player

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func configureProcess(cmd *exec.Cmd, quiet bool) {
	if quiet {
		// Suppress a console window without hiding the player's GUI via SW_HIDE.
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	}
}
