package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"unicode"
)

type Player struct {
	Executable  string
	Arguments   []string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Verbose     *bool
	ConfigError error
}

func (p Player) Play(ctx context.Context, resolvedURL string) error {
	if p.ConfigError != nil {
		return fmt.Errorf("player configuration: %w", p.ConfigError)
	}
	arguments := append(append([]string(nil), p.Arguments...), resolvedURL)
	cmd := exec.CommandContext(ctx, p.Executable, arguments...)
	stdout, stderr := p.Stdout, p.Stderr
	quiet := p.Verbose != nil && !*p.Verbose
	if quiet {
		stdout, stderr = io.Discard, io.Discard
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = p.Stdin, stdout, stderr
	configureProcess(cmd, quiet)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("player %q failed: %w", p.Executable, sanitizeExitError(err))
	}
	return nil
}

func ParseCommand(command string) (string, []string, error) {
	var fields []string
	var current strings.Builder
	var quote rune
	escaped := false
	started := false
	runes := []rune(strings.TrimSpace(command))
	for i, r := range runes {
		if escaped {
			current.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' && i+1 < len(runes) && (unicode.IsSpace(runes[i+1]) || runes[i+1] == '\'' || runes[i+1] == '"') {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			started = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				fields = append(fields, current.String())
				current.Reset()
				started = false
			}
			continue
		}
		current.WriteRune(r)
		started = true
	}
	if quote != 0 || escaped {
		return "", nil, errors.New("unterminated quote or escape")
	}
	if started {
		fields = append(fields, current.String())
	}
	if len(fields) == 0 || fields[0] == "" {
		return "", nil, errors.New("player command is empty")
	}
	return fields[0], fields[1:], nil
}

func sanitizeExitError(err error) error {
	if exit, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("exit status %d", exit.ExitCode())
	}
	return fmt.Errorf("could not start")
}
