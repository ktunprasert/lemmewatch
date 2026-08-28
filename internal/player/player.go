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
	Verbose    *bool
}

func (p Player) Play(ctx context.Context, resolvedURL string) error {
	arguments := append(append([]string(nil), p.Arguments...), resolvedURL)
	cmd := exec.CommandContext(ctx, p.Executable, arguments...)
	stdout, stderr := p.Stdout, p.Stderr
	if p.Verbose != nil && !*p.Verbose {
		stdout, stderr = io.Discard, io.Discard
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = p.Stdin, stdout, stderr
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
