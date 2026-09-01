package cli

import (
	"bytes"
	"strings"
	"testing"

	"lemmewatch/internal/buildinfo"
	"lemmewatch/internal/config"
	"lemmewatch/internal/provider"
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

func TestRootWithoutQueryShowsDashboard(t *testing.T) {
	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("\x1b"))
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "What would you like to watch?") {
		t.Fatalf("unexpected dashboard: %q", out.String())
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

func TestConfiguredAppSelectsWebStreamrWithoutTorBoxToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TORBOX_API_TOKEN", "")
	t.Setenv("LEMMEWATCH_PROVIDER", "")
	original := buildinfo.DefaultTorboxAPIToken
	buildinfo.DefaultTorboxAPIToken = ""
	t.Cleanup(func() { buildinfo.DefaultTorboxAPIToken = original })

	got := configuredApp(new(bool))
	if got.Provider != "webstreamr" || got.Providers[got.Provider] == nil {
		t.Fatalf("provider = %q, providers = %#v", got.Provider, got.ProviderNames)
	}
}

func TestConfiguredAppAddsConfiguredPenguFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TORBOX_API_TOKEN", "")
	t.Setenv("LEMMEWATCH_PROVIDER", "")
	t.Setenv("LEMMEWATCH_PENGUPLAY_MANIFEST_URL", "https://pengu.example/secret/manifest.json")
	original := buildinfo.DefaultTorboxAPIToken
	buildinfo.DefaultTorboxAPIToken = ""
	t.Cleanup(func() { buildinfo.DefaultTorboxAPIToken = original })

	got := configuredApp(new(bool))
	if got.Provider != "webstreamr" || got.Providers[provider.PenguID] == nil || len(got.ProviderNames) != 2 || got.ProviderNames[1] != provider.PenguID {
		t.Fatalf("provider = %q, choices = %#v", got.Provider, got.ProviderNames)
	}
}

func TestConfiguredAppSelectsConfiguredPenguOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LEMMEWATCH_PROVIDER", "pengu")
	t.Setenv("LEMMEWATCH_PENGUPLAY_MANIFEST_URL", "https://pengu.example/secret/manifest.json")
	got := configuredApp(new(bool))
	if got.Provider != provider.PenguID || len(got.ProviderNames) != 1 || got.ProviderNames[0] != provider.PenguID {
		t.Fatalf("provider = %q, choices = %#v", got.Provider, got.ProviderNames)
	}
}

func TestPenguOverrideRequiresConfiguredManifest(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LEMMEWATCH_PROVIDER", "pengu")
	t.Setenv("LEMMEWATCH_PENGUPLAY_MANIFEST_URL", "")
	cmd := New()
	cmd.SetArgs([]string{"search", "Dune"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "LEMMEWATCH_PENGUPLAY_MANIFEST_URL is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestConfiguredAppSelectsTorBoxWhenTokenExists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TORBOX_API_TOKEN", "token")
	t.Setenv("LEMMEWATCH_PROVIDER", "")
	got := configuredApp(new(bool))
	if got.Provider != "torbox" || got.Providers[got.Provider] == nil {
		t.Fatalf("provider = %q, providers = %#v", got.Provider, got.ProviderNames)
	}
}

func TestConfiguredAppProviderEnvironmentOverridesPreference(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TORBOX_API_TOKEN", "token")
	t.Setenv("LEMMEWATCH_PROVIDER", "webstreamr")
	if err := config.Save(config.Preferences{Provider: "torbox"}); err != nil {
		t.Fatal(err)
	}
	got := configuredApp(new(bool))
	if got.Provider != "webstreamr" || len(got.ProviderNames) != 1 || got.ProviderNames[0] != "webstreamr" {
		t.Fatalf("provider = %q, choices = %#v", got.Provider, got.ProviderNames)
	}
}

func TestInvalidProviderOverrideFailsClearly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LEMMEWATCH_PROVIDER", "typo")
	cmd := New()
	cmd.SetArgs([]string{"search", "Dune"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid LEMMEWATCH_PROVIDER") {
		t.Fatalf("error = %v", err)
	}
}

func TestTorBoxOverrideRequiresToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LEMMEWATCH_PROVIDER", "torbox")
	t.Setenv("TORBOX_API_TOKEN", "")
	original := buildinfo.DefaultTorboxAPIToken
	buildinfo.DefaultTorboxAPIToken = ""
	t.Cleanup(func() { buildinfo.DefaultTorboxAPIToken = original })
	cmd := New()
	cmd.SetArgs([]string{"search", "Dune"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "TORBOX_API_TOKEN") {
		t.Fatalf("error = %v", err)
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
