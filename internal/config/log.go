package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var loggedURL = regexp.MustCompile(`https?://\S+`)

func LogFailure(operation string, failure error) error {
	if failure == nil {
		return nil
	}
	filename := LogPath()
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	message := strings.Join(strings.Fields(failure.Error()), " ")
	message = loggedURL.ReplaceAllString(message, "[redacted-url]")
	_, err = fmt.Fprintf(file, "%s %s: %s\n", time.Now().UTC().Format(time.RFC3339), operation, message)
	return err
}

func LogPath() string {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			root = filepath.Join(home, ".local", "state")
		} else {
			root = configRoot()
		}
	}
	return filepath.Join(root, "lemmewatch", "errors.log")
}
