package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/roschreiber/multiclaude/internal/sessions"
)

var (
	orange     = lipgloss.Color("#D97757")
	cream      = lipgloss.Color("#ECE8DF")
	muted      = lipgloss.Color("#8A8580")
	faint      = lipgloss.Color("#4A4744")
	selectedBg = lipgloss.Color("#2B2826")
	ink        = lipgloss.Color("#1A1918")
	green      = lipgloss.Color("#9ECE6A")
	red        = lipgloss.Color("#E06C75")
	yellow     = lipgloss.Color("#F2B35B")
	blue       = lipgloss.Color("#7AA2F7")
	logoShadow = lipgloss.Color("#9C5038")
)

var box = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
var paddedBox = box.Padding(0, 1)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func colored(c lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

func statusColor(status sessions.Status) lipgloss.Color {
	switch status {
	case sessions.Approval:
		return yellow
	case sessions.YourTurn:
		return blue
	case sessions.Working:
		return green
	}
	return muted
}

func statusName(status sessions.Status) string {
	switch status {
	case sessions.Approval:
		return "NEEDS APPROVAL"
	case sessions.YourTurn:
		return "YOUR TURN"
	case sessions.Working:
		return "WORKING"
	}
	return "IDLE"
}

func (m *model) statusIcon(status sessions.Status) string {
	switch status {
	case sessions.Approval:
		return "▲"
	case sessions.YourTurn:
		return "●"
	case sessions.Working:
		return spinner[m.frame%len(spinner)]
	}
	return "○"
}

func timeAgo(d time.Duration) string {
	if d < 10*time.Second {
		return "now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 48*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func (m *model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.splash != nil {
		return m.splashView()
	}
	if m.settingsOpen {
		m.showTerminal(nil)
		return m.settingsView()
	}
	if m.newOpen {
		m.showTerminal(nil)
		return m.newSessionView()
	}

	termWidth, bodyHeight := m.terminalSize()

	selected := m.selected()
	var shown *Terminal
	if selected != nil {
		shown = m.terminals[selected.ID]
	}
	m.showTerminal(shown)

	if m.listChanged || m.cachedSidebar == "" {
		m.buildSidebar()
		m.listChanged = false
	}

	rightBorder := faint
	if m.focused {
		rightBorder = orange
	}

	var right string
	if shown != nil {
		screen := m.terminalScreen(shown, termWidth, bodyHeight)
		right = box.BorderForeground(rightBorder).Width(termWidth).Height(bodyHeight).Render(screen)
	} else {
		info := m.details(selected, termWidth-2, bodyHeight)
		right = paddedBox.BorderForeground(faint).Width(termWidth).Height(bodyHeight).Render(info)
	}

	body := joinSideBySide(m.cachedSidebar, right)
	return m.cachedHeader + "\n" + body + "\n" + m.cachedFooter
}

func (m *model) buildSidebar() {
	list := m.sessionList()
	counts := map[sessions.Status]int{}
	for _, s := range list {
		counts[s.Status(m.scannedAt)]++
	}

	sidebarWidth := m.sidebarWidth()
	_, bodyHeight := m.terminalSize()

	sidebarBorder := orange
	if m.focused {
		sidebarBorder = faint
	}

	sidebarText := m.sidebar(list, counts, sidebarWidth-4, bodyHeight)
	m.cachedSidebar = paddedBox.BorderForeground(sidebarBorder).Width(sidebarWidth - 2).Height(bodyHeight).Render(sidebarText)
	m.cachedHeader = m.header(counts)
	m.cachedFooter = m.footer()
}

func (m *model) showTerminal(shown *Terminal) {
	for _, t := range m.terminals {
		t.setVisible(t == shown)
	}
}

func joinSideBySide(left, right string) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	var out strings.Builder
	for i := range leftLines {
		out.WriteString(leftLines[i])
		if i < len(rightLines) {
			out.WriteString(rightLines[i])
		}
		if i < len(leftLines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
}

func (m *model) header(counts map[sessions.Status]int) string {
	title := lipgloss.NewStyle().Background(orange).Foreground(ink).Bold(true).Padding(0, 1)
	header := " " + title.Render("◆ multiclaude")

	if len(m.terminals) > 0 {
		header += "  " + colored(orange).Render(fmt.Sprintf("◉ %d live", len(m.terminals)))
	}
	for _, status := range []sessions.Status{sessions.Approval, sessions.YourTurn, sessions.Working, sessions.Idle} {
		if counts[status] > 0 {
			text := fmt.Sprintf("%s %d %s", m.statusIcon(status), counts[status], strings.ToLower(statusName(status)))
			header += "  " + colored(statusColor(status)).Render(text)
		}
	}
	return ansi.Truncate(header, m.width, "…")
}

func keyHint(key, description string) string {
	return colored(cream).Bold(true).Render(key) + " " + colored(muted).Render(description)
}

func (m *model) footer() string {
	var parts []string
	if m.focused {
		parts = append(parts, keyHint("ctrl+q", "back to list"))
		parts = append(parts, keyHint("ctrl+↑↓", "switch between live sessions"))
		if s := m.selected(); s != nil && m.terminals[s.ID] != nil && m.terminals[s.ID].scroll > 0 {
			parts = append(parts, colored(yellow).Render("scrolled up · type to jump back"))
		}
	} else if m.filtering {
		parts = append(parts, colored(orange).Render("/ ")+colored(cream).Render(m.filter)+colored(orange).Render("▏"))
		parts = append(parts, keyHint("esc", "clear"))
		parts = append(parts, keyHint("enter", "done"))
	} else {
		parts = append(parts, keyHint("↑↓", "select"))
		parts = append(parts, keyHint("enter", "open"))
		parts = append(parts, keyHint("tab", "next"))
		parts = append(parts, keyHint("n", "new"))
		parts = append(parts, keyHint(",", "settings"))
		parts = append(parts, keyHint("x", "close"))
		parts = append(parts, keyHint("/", "filter"))
		parts = append(parts, keyHint("q", "quit"))
		if m.filter != "" {
			parts = append(parts, colored(orange).Render("filter: "+m.filter))
		}
	}

	footer := " " + strings.Join(parts, colored(faint).Render("  ·  "))
	if m.message != "" && time.Since(m.messageAt) < messageTime {
		footer = " " + colored(green).Render(m.message) + "    " + footer
	}
	return ansi.Truncate(footer, m.width, "…")
}

func (m *model) sidebar(list []*sessions.Session, counts map[sessions.Status]int, width, height int) string {
	if len(list) == 0 {
		return colored(muted).Render("No sessions found.\nPress n to start one.")
	}

	selectedIndex := m.selectedIndex(list)
	selectedLine := 0
	var lines []string
	previous := sessions.Status(-1)

	for i, s := range list {
		status := s.Status(m.scannedAt)

		if status != previous {
			if i > 0 {
				lines = append(lines, "")
			}
			heading := colored(statusColor(status)).Bold(true).Render(statusName(status))
			heading += colored(faint).Render(fmt.Sprintf(" %d", counts[status]))
			lines = append(lines, heading)
			previous = status
		}

		if i == selectedIndex {
			selectedLine = len(lines)
		}
		row1, row2 := m.sessionRows(s, status, i == selectedIndex, width)
		lines = append(lines, row1, row2)
	}

	start := selectedLine - height/3
	start = min(start, len(lines)-height)
	start = max(start, 0)
	end := min(start+height, len(lines))
	return strings.Join(lines[start:end], "\n")
}

func (m *model) sessionRows(s *sessions.Session, status sessions.Status, isSelected bool, width int) (string, string) {
	style := lipgloss.NewStyle()
	bar := " "
	if isSelected {
		style = style.Background(selectedBg)
		bar = "▌"
	}
	bar = style.Foreground(orange).Render(bar)

	icon := style.Foreground(statusColor(status)).Render(" " + m.statusIcon(status) + " ")
	name := ansi.Truncate(s.Name(), width-4, "…")
	name = style.Foreground(cream).Bold(isSelected).Render(name)
	row1 := padRight(bar+icon+name, width, style)

	live := style.Render("   ")
	if m.terminals[s.ID] != nil {
		live = style.Foreground(orange).Render(" ◉ ")
	}
	info := s.Project()
	if s.Branch != "" {
		info += " ⎇ " + s.Branch
	}
	ago := " · " + timeAgo(time.Since(s.Modified))
	info = ansi.Truncate(info, width-4-lipgloss.Width(ago), "…") + ago
	info = style.Foreground(muted).Render(info)
	row2 := padRight(bar+live+info, width, style)

	return row1, row2
}

func padRight(text string, width int, style lipgloss.Style) string {
	missing := width - lipgloss.Width(text)
	if missing <= 0 {
		return text
	}
	return text + style.Render(strings.Repeat(" ", missing))
}

func (m *model) terminalScreen(t *Terminal, width, height int) string {
	lines := t.visibleLines(height)
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}

func (m *model) details(s *sessions.Session, width, height int) string {
	if s == nil {
		return colored(muted).Render("Press n to start a session.")
	}
	status := s.Status(m.scannedAt)

	age := timeAgo(time.Since(s.Modified))
	updated := "updated " + age + " ago"
	if age == "now" {
		updated = "updated just now"
	}
	badge := lipgloss.NewStyle().Background(statusColor(status)).Foreground(ink).Bold(true).Padding(0, 1)
	badgeText := badge.Render(m.statusIcon(status) + " " + statusName(status))

	var lines []string
	lines = append(lines, colored(cream).Bold(true).Width(width).Render(s.Name()))
	lines = append(lines, "")
	lines = append(lines, badgeText+colored(muted).Render("  "+updated+"  ·  enter to resume here"))
	lines = append(lines, "")

	lines = addInfo(lines, "folder", s.Folder, width)
	lines = addInfo(lines, "branch", s.Branch, width)
	if s.Cost > 0 {
		spent := fmt.Sprintf("$%.2f  ", s.Cost)
		spent += colored(green).Render(fmt.Sprintf("+%d", s.Added)) + " "
		spent += colored(red).Render(fmt.Sprintf("−%d", s.Removed))
		lines = addInfo(lines, "spent", spent, width)
	}
	lines = addInfo(lines, "session", s.ID, width)

	if status == sessions.Approval {
		lines = addSection(lines, "Waiting on", s.Tool, 3, width)
	}
	if status == sessions.Working {
		lines = addSection(lines, "Running", s.Tool, 3, width)
	}
	lines = addSection(lines, "You asked", s.Prompt, 5, width)
	lines = addSection(lines, "Recap", s.Recap, 5, width)
	lines = addSection(lines, "Claude said", s.Reply, height, width)

	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func addInfo(lines []string, label, value string, width int) []string {
	if value == "" {
		return lines
	}
	line := colored(muted).Width(9).Render(label) + ansi.Truncate(value, width-9, "…")
	return append(lines, line)
}

func addSection(lines []string, title, text string, maxLines, width int) []string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r", ""))
	if text == "" {
		return lines
	}

	letters := []rune(text)
	mostWeCanShow := (maxLines + 1) * width
	if len(letters) > mostWeCanShow {
		text = string(letters[:mostWeCanShow])
	}

	wrapped := colored(cream).Width(width).Render(text)
	textLines := strings.Split(wrapped, "\n")
	if len(textLines) > maxLines {
		textLines = textLines[:maxLines]
		textLines = append(textLines, colored(muted).Render("…"))
	}

	lines = append(lines, "")
	lines = append(lines, colored(orange).Bold(true).Render(title))
	return append(lines, textLines...)
}
