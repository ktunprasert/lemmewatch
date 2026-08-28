//go:build windows

package player

import (
	"os/exec"
	"testing"
)

func TestConfigureProcessDetachesQuietPlayer(t *testing.T) {
	cmd := exec.Command("player.exe")
	configureProcess(cmd, true)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("process attributes = %#v", cmd.SysProcAttr)
	}
}

func TestConfigureProcessLeavesVerbosePlayerAttached(t *testing.T) {
	cmd := exec.Command("player.exe")
	configureProcess(cmd, false)
	if cmd.SysProcAttr != nil {
		t.Fatalf("process attributes = %#v", cmd.SysProcAttr)
	}
}
