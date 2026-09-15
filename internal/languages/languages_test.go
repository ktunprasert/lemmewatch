package languages

import (
	"slices"
	"testing"
)

func TestOrderedLanguageList(t *testing.T) {
	got, err := ParseList("Japanese, eng, ja, FRE, pt-BR")
	if err != nil || !slices.Equal(got, []string{"ja", "en", "fr", "pt-BR"}) {
		t.Fatalf("normalized = %v, %v", got, err)
	}
	for _, value := range []string{"en,,ja", "unknown", "--no-audio", "und", "en_US", "en,;touch file"} {
		if _, err := ParseList(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	if got, err := ParseList(" "); err != nil || len(got) != 0 {
		t.Fatal("empty should use default")
	}
}

func TestLanguageFallbackRanking(t *testing.T) {
	prefs := []string{"pt-BR", "en"}
	available := [][]string{{"pt-BR"}, {"por"}, {"eng"}, nil, {"ja"}}
	for i := 1; i < len(available); i++ {
		if Rank(available[i-1], prefs) >= Rank(available[i], prefs) {
			t.Fatalf("fallback order failed: %v", available)
		}
	}
	if Rank([]string{"ja"}, nil) != Rank(nil, nil) {
		t.Fatal("unset preference affects rank")
	}
}
