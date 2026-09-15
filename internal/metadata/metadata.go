// Package metadata retains provider fields for display without retaining
// credentials, request headers, or temporary playback URLs.
package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
)

type Fields map[string]json.RawMessage

var urlPattern = regexp.MustCompile(`(?i)(?:https?|ftp|magnet|stremio)://\S+|magnet:\?\S+`)

// Parse keeps unknown fields too. Omitted fields belong to separate child items
// (for example, a show's videos) or the provider's private transport metadata.
func Parse(data []byte, omit ...string) Fields {
	var fields Fields
	if json.Unmarshal(data, &fields) != nil {
		return nil
	}
	result := make(Fields)
	for key, raw := range fields {
		if slices.Contains(omit, key) || privateKey(key) {
			continue
		}
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			continue
		}
		clean := sanitize(value)
		if clean == nil {
			continue
		}
		result[key], _ = json.Marshal(clean)
	}
	return result
}

func privateKey(key string) bool {
	key = strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, key))
	for _, part := range []string{"token", "password", "secret", "authorization", "apikey", "credential", "cookie", "header", "authid", "signed", "url", "uri"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return key == "auth" || key == "sources" || key == "s3path" || key == "webdav" || key == "downloadlink"
}

func sanitize(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, child := range value {
			if !privateKey(key) {
				if clean := sanitize(child); clean != nil {
					result[key] = clean
				}
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	case []any:
		result := make([]any, 0, len(value))
		for _, child := range value {
			if clean := sanitize(child); clean != nil {
				result = append(result, clean)
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	case string:
		return urlPattern.ReplaceAllString(value, "[url omitted]")
	default:
		return value
	}
}

// Lines flattens nested objects deterministically. Scalars in arrays share a
// line; object arrays have numbered paths so every retained field is accessible.
func Lines(source string, fields Fields, omit ...string) []string {
	var result []string
	keys := make([]string, 0, len(fields))
	for key := range fields {
		if !slices.Contains(omit, key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(fields[key]))
		decoder.UseNumber()
		if decoder.Decode(&value) == nil {
			result = append(result, flatten(label(key), value)...)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return append([]string{source + ":"}, result...)
}

func flatten(path string, value any) []string {
	switch value := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var result []string
		for _, key := range keys {
			result = append(result, flatten(path+" > "+label(key), value[key])...)
		}
		return result
	case []any:
		var scalars, result []string
		for i, child := range value {
			switch child.(type) {
			case map[string]any, []any:
				result = append(result, flatten(fmt.Sprintf("%s [%d]", path, i+1), child)...)
			default:
				if text := scalar(child); text != "" {
					scalars = append(scalars, text)
				}
			}
		}
		if len(scalars) > 0 {
			result = append([]string{path + ": " + strings.Join(scalars, ", ")}, result...)
		}
		return result
	default:
		if text := scalar(value); text != "" {
			return []string{path + ": " + text}
		}
		return nil
	}
}

func scalar(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func label(value string) string {
	var out []rune
	for i, r := range []rune(value) {
		if r == '_' || r == '-' {
			r = ' '
		}
		if i > 0 && unicode.IsUpper(r) && len(out) > 0 && unicode.IsLower(out[len(out)-1]) {
			out = append(out, ' ')
		}
		if i == 0 {
			r = unicode.ToUpper(r)
		}
		out = append(out, r)
	}
	return strings.Join(strings.Fields(string(out)), " ")
}
