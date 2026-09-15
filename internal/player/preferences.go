package player

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"lemmewatch/internal/languages"
)

func (p Player) preferenceArguments() []string {
	name := p.name()
	if name != "mpv" && name != "vlc" {
		return nil
	}
	var args []string
	for _, setting := range []struct {
		values           []string
		mpvFlag, vlcFlag string
		audio            bool
	}{
		{p.Preferences.AudioLanguages, "--alang=", "--audio-language=", true},
		{p.Preferences.SubtitleLanguages, "--slang=", "--sub-language=", false},
	} {
		var values []string
		for _, value := range setting.values {
			code, err := languages.Normalize(value)
			if err != nil {
				continue
			}
			// VLC accepts ISO language codes, not region tags.
			if name == "vlc" {
				code = strings.Split(code, "-")[0]
			}
			if !slices.Contains(values, code) {
				values = append(values, code)
			}
		}
		if len(values) == 0 {
			continue
		}
		if name == "mpv" {
			args = append(args, setting.mpvFlag+strings.Join(values, ","))
			if setting.audio {
				args = append(args, "--aid=auto")
			} else {
				args = append(args, "--sid=auto", "--sub-visibility=yes", "--subs-with-matching-audio=yes")
			}
		} else {
			// "any" retains VLC's normal track fallback after preferred languages.
			args = append(args, setting.vlcFlag+strings.Join(append(values, "any"), ","))
			if setting.audio {
				args = append(args, "--audio-track=-1", "--audio-track-id=-1", "--audio")
			} else {
				args = append(args, "--sub-track=-1", "--sub-track-id=-1", "--spu")
			}
		}
	}
	speed := p.Preferences.PlaybackSpeed
	if !math.IsNaN(speed) && !math.IsInf(speed, 0) && speed >= 0.25 && speed <= 4 {
		flag := "--speed="
		if name == "vlc" {
			flag = "--rate="
		}
		args = append(args, flag+strconv.FormatFloat(speed, 'f', -1, 64))
	}
	return args
}
