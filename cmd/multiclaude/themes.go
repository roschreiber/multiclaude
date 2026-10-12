package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	Accent     string
	Text       string
	Muted      string
	Faint      string
	Selected   string
	Ink        string
	Green      string
	Red        string
	Yellow     string
	Blue       string
	LogoShadow string
}

var themeNames = []string{"claude", "midnight", "gruvbox", "solarized", "nord", "catppuccin", "dracula", "rose pine", "paper"}

var themes = map[string]Theme{
	"claude":    {"#D97757", "#ECE8DF", "#8A8580", "#4A4744", "#2B2826", "#1A1918", "#9ECE6A", "#E06C75", "#F2B35B", "#7AA2F7", "#9C5038"},
	"midnight":  {"#7AA2F7", "#C0CAF5", "#7A82A8", "#3B4261", "#1F2335", "#15161E", "#9ECE6A", "#F7768E", "#E0AF68", "#7DCFFF", "#3D59A1"},
	"gruvbox":   {"#FE8019", "#EBDBB2", "#A89984", "#504945", "#3C3836", "#1D2021", "#B8BB26", "#FB4934", "#FABD2F", "#83A598", "#AF5A12"},
	"solarized": {"#CB4B16", "#EEE8D5", "#93A1A1", "#586E75", "#073642", "#002B36", "#859900", "#DC322F", "#B58900", "#268BD2", "#8A3410"},
	"nord":      {"#88C0D0", "#ECEFF4", "#8F99AB", "#434C5E", "#3B4252", "#2E3440", "#A3BE8C", "#BF616A", "#EBCB8B", "#81A1C1", "#4C7480"},
	"paper":     {"#C4552F", "#3B3A36", "#7A756C", "#BDB6A6", "#E8E2D2", "#FFFFFF", "#4E8A1E", "#C2362E", "#B87500", "#2F5FD0", "#E0A38E"},

	// catppuccin mocha, https://catppuccin.com/palette
	"catppuccin": {"#CBA6F7", "#CDD6F4", "#7F849C", "#45475A", "#313244", "#11111B", "#A6E3A1", "#F38BA8", "#F9E2AF", "#89B4FA", "#585B70"},
	// https://draculatheme.com/spec
	"dracula": {"#FF79C6", "#F8F8F2", "#6272A4", "#44475A", "#44475A", "#282A36", "#50FA7B", "#FF5555", "#F1FA8C", "#8BE9FD", "#BD93F9"},
	// https://rosepinetheme.com/palette
	"rose pine": {"#EBBCBA", "#E0DEF4", "#908CAA", "#403D52", "#26233A", "#191724", "#9CCFD8", "#EB6F92", "#F6C177", "#C4A7E7", "#EB6F92"},
}

func applyTheme(name string) {
	t, found := themes[name]
	if !found {
		t = themes["claude"]
	}
	orange = lipgloss.Color(t.Accent)
	cream = lipgloss.Color(t.Text)
	muted = lipgloss.Color(t.Muted)
	faint = lipgloss.Color(t.Faint)
	selectedBg = lipgloss.Color(t.Selected)
	ink = lipgloss.Color(t.Ink)
	green = lipgloss.Color(t.Green)
	red = lipgloss.Color(t.Red)
	yellow = lipgloss.Color(t.Yellow)
	blue = lipgloss.Color(t.Blue)
	logoShadow = lipgloss.Color(t.LogoShadow)
	writeCCTheme(name, t)
}

func CCPath() string {
	return filepath.Join(filepath.Dir(configPath()), "claude-settings.json")
}

func writeCCTheme(name string, t Theme) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	base := "dark"
	if name == "paper" { 
		base = "light"
	}
	data, _ := json.MarshalIndent(map[string]any{
		"name": "multiclaude",
		"base": base,
		"overrides": map[string]string{
			"claude": t.Accent, "clawd_body": t.Accent, "text": t.Text, "inactive": t.Muted, "subtle": t.Faint,
			"promptBorder": t.Muted, "inverseText": t.Ink,
			"success": t.Green, "error": t.Red, "warning": t.Yellow,
			"suggestion": t.Blue, "planMode": t.Blue,
		},
	}, "", "  ")
	dir := filepath.Join(home, ".claude", "themes")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "multiclaude.json"), data, 0o644)

	os.MkdirAll(filepath.Dir(CCPath()), 0o755)
	os.WriteFile(CCPath(), []byte(`{"theme":"custom:multiclaude"}`), 0o644)
}
