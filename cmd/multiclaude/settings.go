package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type Setting struct {
	Key     string
	Label   string
	Help    string
	Options []string
	Default string
}

var settingsList = []Setting{
	{"theme", "Theme", "Change the main colors of the app", themeNames, "claude"},
	{"intro", "Intro animation", "When to show the Clawd intro: every start, only the first time, or never", []string{"always", "first run", "off"}, "first run"},
	{"sidebar", "Sidebar width", "How many columns the session list may use, at most", []string{"30", "36", "40", "46", "54", "60"}, "46"},
	{"rescan", "Rescan interval", "Interval between looking for new Claude Code Activity", []string{"1", "2", "5", "10"}, "2"},
	{"devcontainers", "Devcontainers", "New sessions in a folder with a devcontainer run inside its running container. Off: always run on this computer", []string{"auto", "off"}, "auto"},
	{"confirmQuit", "Confirm quit", "Press q twice to quit while sessions are running", []string{"on", "off"}, "on"},
}

var config = map[string]string{}

func findSetting(key string) *Setting {
	for i := range settingsList {
		if settingsList[i].Key == key {
			return &settingsList[i]
		}
	}
	return nil
}

func indexOf(list []string, value string) int {
	for i, item := range list {
		if item == value {
			return i
		}
	}
	return -1
}

func setting(key string) string {
	value := config[key]
	st := findSetting(key)
	if st == nil {
		return value
	}
	if indexOf(st.Options, value) == -1 {
		return st.Default
	}
	return value
}

func settingNumber(key string) int {
	number, _ := strconv.Atoi(setting(key))
	return number
}

func settingOn(key string) bool {
	return setting(key) == "on"
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir, _ = os.UserHomeDir()
	}
	return filepath.Join(dir, "multiclaude", "settings.json")
}

func loadConfig() {
	config = map[string]string{}
	data, err := os.ReadFile(configPath())
	if err == nil {
		json.Unmarshal(data, &config)
	}
}

func saveConfig() error {
	err := os.MkdirAll(filepath.Dir(configPath()), 0o755)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0o644)
}

func (m *model) changeSetting(step int) {
	st := settingsList[m.settingsRow]
	current := indexOf(st.Options, setting(st.Key))
	next := (current + step + len(st.Options)) % len(st.Options)
	config[st.Key] = st.Options[next]

	if st.Key == "theme" {
		applyTheme(config["theme"])
	}
	if st.Key == "sidebar" {
		m.resizeTerminals()
	}

	err := saveConfig()
	if err != nil {
		m.showMessage("× could not save settings: " + err.Error())
	}
}

func (m *model) handleSettingsKey(key tea.KeyMsg) {
	switch key.String() {
	case "esc", "q", ",", "ctrl+c":
		m.settingsOpen = false
	case "up", "k":
		m.settingsRow = max(m.settingsRow-1, 0)
	case "down", "j":
		m.settingsRow = min(m.settingsRow+1, len(settingsList)-1)
	case "left", "h":
		m.changeSetting(-1)
	case "right", "l", "enter", " ":
		m.changeSetting(1)
	}
}

func (m *model) settingsView() string {
	width := max(20, min(72, m.width-6))
	inner := width - 4

	lines := []string{
		colored(orange).Bold(true).Render("◆ settings"),
		colored(muted).Render(ansi.Truncate("saved in "+configPath(), inner, "…")),
		"",
	}

	for i, st := range settingsList {
		bar := "  "
		label := colored(muted)
		value := colored(cream).Render(setting(st.Key))
		if i == m.settingsRow {
			bar = colored(orange).Render("▌ ")
			label = colored(cream).Bold(true)
			value = colored(orange).Bold(true).Render("‹ " + setting(st.Key) + " ›")
		}
		line := bar + label.Width(18).Render(st.Label) + value
		lines = append(lines, ansi.Truncate(line, inner, "…"))
	}

	swatch := ""
	colors := []lipgloss.Color{orange, green, yellow, red, blue, cream, muted, faint}
	for _, c := range colors {
		swatch += colored(c).Render("███ ")
	}
	lines = append(lines, "", "  "+swatch, "")
	lines = append(lines, colored(muted).Width(inner).Render(settingsList[m.settingsRow].Help))

	if m.message != "" && time.Since(m.messageAt) < messageTime {
		lines = append(lines, "", colored(red).Render(m.message))
	}

	hints := keyHint("↑↓", "select") + "  " + keyHint("←→", "change") + "  " + keyHint("esc", "back")
	lines = append(lines, "", hints)

	page := paddedBox.BorderForeground(orange).Width(width).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, page)
}
