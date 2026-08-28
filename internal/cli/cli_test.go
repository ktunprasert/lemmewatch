package cli

import (
	"bytes"
	"strings"
	"testing"

	"lemmewatch/internal/buildinfo"
	"lemmewatch/internal/config"
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

func TestVersionShowsBuildCommit(t *testing.T) {
	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "lemmewatch development") {
		t.Fatalf("version = %q", out.String())
	}
}

func TestQueryFlagTreatsCommandNameAsFlagValue(t *testing.T) {
	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--query", "history", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Find and stream media") || strings.Contains(out.String(), "List recently played titles\n\nUsage:") {
		t.Fatalf("query flag resolved to subcommand help: %q", out.String())
	}
}

func TestQueryFlagPreservesAdditionalWords(t *testing.T) {
	if got := forcedQueryText("Dune", []string{"Part", "Two"}); got != "Dune Part Two" {
		t.Fatalf("query = %q", got)
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

func TestConfiguredAppUsesEmbeddedTorboxTokenAsFallback(t *testing.T) {
	t.Setenv("TORBOX_API_TOKEN", "")
	original := buildinfo.DefaultTorboxAPIToken
	buildinfo.DefaultTorboxAPIToken = "embedded-token"
	t.Cleanup(func() { buildinfo.DefaultTorboxAPIToken = original })

	if got := configuredApp(new(bool)).TorBox.Token; got != "embedded-token" {
		t.Fatalf("token = %q", got)
	}
}

func TestConfiguredAppPrefersEnvironmentTorboxToken(t *testing.T) {
	t.Setenv("TORBOX_API_TOKEN", "environment-token")
	original := buildinfo.DefaultTorboxAPIToken
	buildinfo.DefaultTorboxAPIToken = "embedded-token"
	t.Cleanup(func() { buildinfo.DefaultTorboxAPIToken = original })

	if got := configuredApp(new(bool)).TorBox.Token; got != "environment-token" {
		t.Fatalf("token = %q", got)
	}
}

func TestConfiguredAppUsesPreferredPlayer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LEMMEWATCH_PLAYER", "")
	if err := config.Save(config.Preferences{Player: "vlc --no-video-title-show"}); err != nil {
		t.Fatal(err)
	}
	got := configuredApp(new(bool)).Player
	if got.Executable != "vlc" || len(got.Arguments) != 1 || got.Arguments[0] != "--no-video-title-show" {
		t.Fatalf("player = %#v", got)
	}
}

func TestConfiguredAppEnvironmentOverridesPreferredPlayer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LEMMEWATCH_PLAYER", "custom-player")
	if err := config.Save(config.Preferences{Player: "vlc"}); err != nil {
		t.Fatal(err)
	}
	if got := configuredApp(new(bool)).Player.Executable; got != "custom-player" {
		t.Fatalf("player = %q", got)
	}
}

func TestDebugEnvironmentEnablesDiagnostics(t *testing.T) {
	t.Setenv("DEBUG", "yes")
	if !envEnabled("DEBUG") {
		t.Fatal("DEBUG did not enable diagnostics")
	}
	t.Setenv("DEBUG", "0")
	if envEnabled("DEBUG") {
		t.Fatal("DEBUG=0 enabled diagnostics")
	}
}
