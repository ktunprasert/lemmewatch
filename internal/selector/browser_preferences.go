package selector

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"lemmewatch/internal/languages"
	"lemmewatch/internal/model"
)

func languageSettingLabel(values []string) string {
	if len(values) == 0 {
		return "Default"
	}
	return strings.Join(values, ", ")
}

func speedSettingLabel(speed float64) string {
	if speed == 0 {
		return "Default"
	}
	return strconv.FormatFloat(speed, 'f', -1, 64) + "x"
}

func (m browserModel[T]) playbackSettingText() string {
	switch m.settingsIndex {
	case 11:
		return strings.Join(m.playbackPreferences.AudioLanguages, ",")
	case 12:
		return strings.Join(m.playbackPreferences.SubtitleLanguages, ",")
	default:
		if m.playbackPreferences.PlaybackSpeed == 0 {
			return ""
		}
		return strconv.FormatFloat(m.playbackPreferences.PlaybackSpeed, 'f', -1, 64)
	}
}

func (m *browserModel[T]) savePlaybackPreferences(preferences model.PlaybackPreferences) bool {
	if m.options.SavePlayback != nil {
		if err := m.options.SavePlayback(preferences); err != nil {
			m.saveSetting(err)
			return false
		}
	}
	m.playbackPreferences = preferences
	m.right.index = 0
	return true
}

func (m browserModel[T]) updatePlaybackSetting(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeOverlay()
	case "enter":
		preferences := m.playbackPreferences
		value := strings.TrimSpace(m.playbackSettingValue)
		var err error
		if m.settingsIndex == 13 {
			var speed float64
			if value != "" {
				speed, err = strconv.ParseFloat(strings.TrimSuffix(value, "x"), 64)
			}
			if err != nil || math.IsNaN(speed) || math.IsInf(speed, 0) || value != "" && (speed < 0.25 || speed > 4) {
				err = fmt.Errorf("speed must be 0.25–4; clear for default")
			}
			preferences.PlaybackSpeed = speed
		} else {
			var values []string
			values, err = languages.ParseList(value)
			if m.settingsIndex == 11 {
				preferences.AudioLanguages = values
			} else {
				preferences.SubtitleLanguages = values
			}
		}
		if err != nil {
			m.toasts.Err(ToastSettings, "%s", err)
			return m, nil
		}
		if m.savePlaybackPreferences(preferences) {
			m.closeOverlay()
		}
	case "backspace", "ctrl+h":
		runes := []rune(m.playbackSettingValue)
		if len(runes) > 0 {
			m.playbackSettingValue = string(runes[:len(runes)-1])
		}
	case "ctrl+u":
		m.playbackSettingValue = ""
	case "ctrl+c":
		return m, tea.Quit
	default:
		if msg.Type == tea.KeySpace {
			m.playbackSettingValue += " "
		} else if msg.Type == tea.KeyRunes {
			m.playbackSettingValue += string(msg.Runes)
		}
	}
	return m, nil
}
