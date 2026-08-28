package player

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPlayDoesNotExposeURL(t *testing.T) {
	secret := "https://example.invalid/file?token=very-secret"
	err := (Player{Executable: "/definitely/missing/player"}).Play(context.Background(), secret)
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
			if err := p.Play(context.Background(), "https://example.invalid/video"); err != nil {
				t.Fatal(err)
			}
			if got := output.String() + errors.String(); got != test.want {
				t.Fatalf("output = %q", got)
			}
		})
	}
}
