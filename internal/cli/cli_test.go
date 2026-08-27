package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpForms(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {"help", "search"}, {"search", "--help"}} {
		cmd := New()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(strings.ToLower(out.String()), "usage") {
			t.Fatalf("%v produced no usage: %q", args, out.String())
		}
	}
}

func TestRootWithoutQueryShowsHelp(t *testing.T) {
	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Available Commands") {
		t.Fatalf("unexpected help: %q", out.String())
	}
}

func TestDefaultPlayerUsesPlatformOpener(t *testing.T) {
	tests := map[string]struct {
		executable string
		argument   string
	}{
		"darwin":  {executable: "open"},
		"linux":   {executable: "xdg-open"},
		"windows": {executable: "rundll32", argument: "url.dll,FileProtocolHandler"},
	}
	for goos, expected := range tests {
		executable, arguments := defaultPlayer(goos)
		if executable != expected.executable {
			t.Errorf("defaultPlayer(%q) executable = %q, want %q", goos, executable, expected.executable)
		}
		if expected.argument != "" && (len(arguments) != 1 || arguments[0] != expected.argument) {
			t.Errorf("defaultPlayer(%q) arguments = %#v", goos, arguments)
		}
	}
}
