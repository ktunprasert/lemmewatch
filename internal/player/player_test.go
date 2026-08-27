package player

import (
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
