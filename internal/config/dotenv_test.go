package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotenv(t *testing.T) {
	filename := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(filename, []byte("DOTENV_NEW='new value'\nDOTENV_EXISTING=dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_EXISTING", "process")
	os.Unsetenv("DOTENV_NEW")
	t.Cleanup(func() { os.Unsetenv("DOTENV_NEW") })
	if err := LoadDotenv(filename); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DOTENV_NEW"); got != "new value" {
		t.Fatalf("new value = %q", got)
	}
	if got := os.Getenv("DOTENV_EXISTING"); got != "process" {
		t.Fatalf("existing value overridden: %q", got)
	}
}

func TestLoadDotenvIgnoresMissingFile(t *testing.T) {
	if err := LoadDotenv(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatal(err)
	}
}
