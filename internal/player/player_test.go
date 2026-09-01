package player

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"lemmewatch/internal/model"
)

func TestPlayDoesNotExposeURL(t *testing.T) {
	secret := "https://example.invalid/file?token=very-secret"
	err := (Player{Executable: "/definitely/missing/player"}).Play(context.Background(), model.Playback{URL: secret})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "very-secret") {
		t.Fatalf("secret leaked: %v", err)
	}
}

func TestPlaySuppressesOutputUnlessVerbose(t *testing.T) {
	for _, test := range []struct {
		name    string
		verbose bool
		want    string
	}{
		{name: "quiet"},
		{name: "verbose", verbose: true, want: "outputerror"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output, errors bytes.Buffer
			p := Player{Executable: "/bin/sh", Arguments: []string{"-c", "printf output; printf error >&2"}, Stdout: &output, Stderr: &errors, Verbose: &test.verbose}
			if err := p.Play(context.Background(), model.Playback{URL: "https://example.invalid/video"}); err != nil {
				t.Fatal(err)
			}
			if got := output.String() + errors.String(); got != test.want {
				t.Fatalf("output = %q", got)
			}
		})
	}
}

func TestPlayRejectsRequestHeaders(t *testing.T) {
	err := (Player{Executable: "/bin/true"}).Play(context.Background(), model.Playback{URL: "https://example.invalid/video", Headers: map[string]string{"Referer": "https://example.invalid/"}})
	if err == nil || !strings.Contains(err.Error(), "headers") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseCommandSupportsQuotedArguments(t *testing.T) {
	executable, arguments, err := ParseCommand(`"C:\Program Files\mpv\mpv.exe" --no-border --title "My Player"`)
	if err != nil {
		t.Fatal(err)
	}
	if executable != `C:\Program Files\mpv\mpv.exe` || strings.Join(arguments, "|") != "--no-border|--title|My Player" {
		t.Fatalf("command = %q %#v", executable, arguments)
	}
}

func TestParseCommandPreservesUNCPath(t *testing.T) {
	executable, _, err := ParseCommand(`"\\server\share\mpv.exe" --fullscreen`)
	if err != nil {
		t.Fatal(err)
	}
	if executable != `\\server\share\mpv.exe` {
		t.Fatalf("executable = %q", executable)
	}
}

func TestParseCommandRejectsMalformedInput(t *testing.T) {
	for _, command := range []string{"", `mpv --title "unfinished`} {
		if _, _, err := ParseCommand(command); err == nil {
			t.Fatalf("ParseCommand(%q) succeeded", command)
		}
	}
}
