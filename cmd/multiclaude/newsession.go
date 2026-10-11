package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) openNewSession() {
	input := textinput.New()
	input.Prompt = "> "
	input.Width = max(10, min(60, m.width-16))
	input.SetValue(startFolder)
	input.CursorEnd()
	input.Focus()

	m.newInput = input
	m.newOpen = true
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "\"")
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}
	return path
}

func (m *model) handleNewKey(key tea.KeyMsg) {
	if key.Type == tea.KeyEsc {
		m.newOpen = false
		return
	}

	if key.Type == tea.KeyEnter {
		path := cleanPath(m.newInput.Value())
		err := m.open(path, newUUID(), false)
		if err == nil {
			m.newOpen = false
		}
		return
	}

	m.newInput, _ = m.newInput.Update(key)
}

func (m *model) newSessionView() string {
	width := max(20, min(72, m.width-6))

	lines := []string{
		colored(orange).Bold(true).Render("◆ new session"),
		colored(muted).Render("Folder to start claude in:"),
		"",
		m.newInput.View(),
	}

	if m.message != "" && time.Since(m.messageAt) < messageTime {
		lines = append(lines, "", colored(red).Render(m.message))
	}

	hints := keyHint("enter", "start") + "  " + keyHint("esc", "cancel")
	lines = append(lines, "", hints)

	page := paddedBox.BorderForeground(orange).Width(width).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, page)
}
