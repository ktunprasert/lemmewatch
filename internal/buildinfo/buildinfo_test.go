package buildinfo

import "testing"

func TestCommitHasDevelopmentFallback(t *testing.T) {
	if Commit == "" {
		t.Fatal("commit must not be empty")
	}
}
