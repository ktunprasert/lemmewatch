package stremio

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"lemmewatch/internal/languages"
)

var languageSectionRE = regexp.MustCompile(`(?i)\b(audio(?: languages?)?|subtitles?|subs?)\s*[:=]\s*`)

// Release flags are language hints, not verified audio/subtitle track metadata.
// Explicit labeled sections are kept separate so "Subs: English" cannot claim
// that the release has English audio. MULTI/DUAL alone imply no specific language.
func releaseLanguages(text string) (audio, subtitles, hints []string) {
	for line := range strings.SplitSeq(text, "\n") {
		sections := languageSectionRE.FindAllStringSubmatchIndex(line, -1)
		if len(sections) == 0 {
			hints = mergeLanguages(hints, detectLanguages(line, false))
			continue
		}
		hints = mergeLanguages(hints, detectLanguages(line[:sections[0][0]], false))
		for i, section := range sections {
			end := len(line)
			if i+1 < len(sections) {
				end = sections[i+1][0]
			}
			found := detectLanguages(line[section[1]:end], true)
			if strings.HasPrefix(strings.ToLower(line[section[2]:section[3]]), "audio") {
				audio = mergeLanguages(audio, found)
			} else {
				subtitles = mergeLanguages(subtitles, found)
			}
		}
	}
	return
}

func detectLanguages(text string, explicit bool) []string {
	var result []string
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) })
	for _, choice := range languages.Choices {
		found := slices.Contains(words, strings.ToLower(choice.Name))
		for _, flag := range strings.Fields(choice.Flags) {
			found = found || strings.Contains(text, flag)
		}
		if found {
			result = mergeLanguages(result, []string{choice.Code})
		}
	}
	for _, word := range words {
		// Bare two-letter words occur in titles (It, Us, ...); only accept them
		// in labeled metadata. Three-letter ISO codes are common release tags.
		if !explicit && len(word) != 3 {
			continue
		}
		if code, err := languages.Normalize(word); err == nil {
			if explicit || slices.ContainsFunc(languages.Choices, func(choice languages.Language) bool { return choice.Code == code }) {
				result = mergeLanguages(result, []string{code})
			}
		}
	}
	return result
}

func mergeLanguages(a, b []string) []string {
	for _, value := range b {
		if !slices.Contains(a, value) {
			a = append(a, value)
		}
	}
	return a
}
