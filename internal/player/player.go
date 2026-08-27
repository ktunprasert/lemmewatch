package player

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

type Player struct {
	Executable string
	Arguments  []string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
}

func (p Player) Play(ctx context.Context, resolvedURL string) error {
	arguments := append(append([]string(nil), p.Arguments...), resolvedURL)
	cmd := exec.CommandContext(ctx, p.Executable, arguments...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = p.Stdin, p.Stdout, p.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("player %q failed: %w", p.Executable, sanitizeExitError(err))
	}
	return nil
}

func sanitizeExitError(err error) error {
	if exit, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("exit status %d", exit.ExitCode())
	}
	return fmt.Errorf("could not start")
}
