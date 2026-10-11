package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/vt"

	"github.com/roschreiber/multiclaude/internal/devcontainer"
	"github.com/roschreiber/multiclaude/internal/sessions"
)

var program *tea.Program

var startFolder string

const messageTime = 6 * time.Second

type model struct {
	root      string
	cache     map[string]*sessions.Session
	scanned   []*sessions.Session
	scannedAt time.Time
	terminals map[string]*Terminal

	selectedID string
	focused    bool
	filtering  bool
	filter     string
	confirm    string
	message    string
	messageAt  time.Time
	width      int
	height     int
	frame      int
	splash     *splash

	settingsOpen bool
	settingsRow  int

	newOpen  bool
	newInput textinput.Model

	listChanged   bool
	cachedHeader  string
	cachedSidebar string
	cachedFooter  string
	oldTitle      string
}

type scanDoneMsg struct{ list []*sessions.Session }
type tickMsg struct{}

func (m *model) scanLater(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return scanDoneMsg{list: sessions.Scan(m.root, m.cache)}
	})
}

func (m *model) tick() tea.Cmd {
	delay := time.Second
	for _, s := range m.scanned {
		if s.Status(m.scannedAt) == sessions.Working {
			delay = 100 * time.Millisecond
			break
		}
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.scanLater(0), m.tick(), splashTick())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	output, isOutput := msg.(outputMsg)
	if isOutput {
		return m, output.terminal.redrawDone()
	}
	m.listChanged = true

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeTerminals()
	case scanDoneMsg:
		m.scanned = msg.list
		m.scannedAt = time.Now()
		delay := time.Duration(settingNumber("rescan")) * time.Second
		title := m.windowTitle()
		if title == m.oldTitle {
			return m, m.scanLater(delay)
		}
		m.oldTitle = title
		return m, tea.Batch(m.scanLater(delay), tea.SetWindowTitle(title))
	case tickMsg:
		m.frame++
		return m, m.tick()
	case exitedMsg:
		m.terminalExited(msg.terminal)
	case splashTickMsg:
		if m.splash == nil {
			return m, nil
		}
		if m.width > 0 {
			m.splash.step(m.width)
		}
		if m.splash.done(m.width) {
			m.splash = nil
			return m, nil
		}
		return m, splashTick()
	case tea.MouseMsg:
		m.handleMouse(msg)
	case tea.KeyMsg:
		if m.splash != nil {
			m.splash = nil
			return m, nil
		}
		return m, m.handleKey(msg)
	}
	return m, nil
}

func (m *model) sidebarWidth() int {
	return min(settingNumber("sidebar"), m.width*2/5)
}

func (m *model) resizeTerminals() {
	width, height := m.terminalSize()
	for _, t := range m.terminals {
		t.Resize(width, height)
	}
}

func (m *model) terminalSize() (int, int) {
	width := max(1, m.width-m.sidebarWidth()-2)
	height := max(1, m.height-4)
	return width, height
}

func (m *model) sessionList() []*sessions.Session {
	all := append([]*sessions.Session{}, m.scanned...)
	for id, t := range m.terminals {
		if !hasSession(m.scanned, id) {
			newSession := &sessions.Session{
				ID:        id,
				Folder:    t.Folder,
				Title:     "New session",
				Modified:  t.Started,
				LastEvent: "reply",
			}
			all = append(all, newSession)
		}
	}

	var list []*sessions.Session
	search := strings.ToLower(m.filter)
	for _, s := range all {
		text := strings.ToLower(s.Name() + " " + s.Project() + " " + s.Branch + " " + s.Prompt)
		if strings.Contains(text, search) {
			list = append(list, s)
		}
	}

	sessions.Sort(list, m.scannedAt)
	return list
}

func hasSession(list []*sessions.Session, id string) bool {
	for _, s := range list {
		if s.ID == id {
			return true
		}
	}
	return false
}

func (m *model) selectedIndex(list []*sessions.Session) int {
	for i, s := range list {
		if s.ID == m.selectedID {
			return i
		}
	}
	return 0
}

func (m *model) selected() *sessions.Session {
	list := m.sessionList()
	if len(list) == 0 {
		return nil
	}
	return list[m.selectedIndex(list)]
}

func (m *model) moveSelection(step int) {
	list := m.sessionList()
	if len(list) == 0 {
		return
	}
	i := m.selectedIndex(list) + step
	i = max(i, 0)
	i = min(i, len(list)-1)
	m.selectedID = list[i].ID
}

func (m *model) isWaiting(s *sessions.Session) bool {
	status := s.Status(m.scannedAt)
	return status == sessions.Approval || status == sessions.YourTurn
}

func (m *model) selectNext() {
	list := m.sessionList()
	if len(list) == 0 {
		return
	}
	next := (m.selectedIndex(list) + 1) % len(list)
	m.selectedID = list[next].ID
}

func (m *model) switchLive(step int) {
	var live []*sessions.Session
	for _, s := range m.sessionList() {
		if m.terminals[s.ID] != nil {
			live = append(live, s)
		}
	}
	if len(live) == 0 {
		m.showMessage("no live sessions")
		return
	}

	i := -1
	if hasSession(live, m.selectedID) {
		i = m.selectedIndex(live)
	}
	i = i + step
	if i < 0 {
		i = len(live) - 1
	}
	if i >= len(live) {
		i = 0
	}
	m.selectedID = live[i].ID
	m.focused = true
}

func (m *model) windowTitle() string {
	waiting := 0
	for _, s := range m.sessionList() {
		if m.isWaiting(s) {
			waiting++
		}
	}
	if waiting == 0 {
		return "multiclaude"
	}
	return fmt.Sprintf("multiclaude (%d waiting)", waiting)
}

func (m *model) showMessage(text string) {
	m.message = text
	m.messageAt = time.Now()
}

func (m *model) open(folder, id string, resume bool) error {
	if m.terminals[id] == nil {
		container, workFolder, err := devcontainer.ResolveFolder(folder, resume, setting("devcontainers") != "off")
		if err != nil {
			m.showMessage("× " + err.Error())
			return err
		}
		width, height := m.terminalSize()
		t, err := startTerminal(container, workFolder, id, resume, width, height)
		if err != nil {
			m.showMessage("× " + err.Error())
			return err
		}
		m.terminals[id] = t
		m.selectedID = id
		m.filter = ""
		if container != nil {
			m.showMessage("running in devcontainer " + container.Name)
		}
	}
	m.focused = true
	return nil
}

func (m *model) terminalExited(t *Terminal) {
	if m.terminals[t.ID] != t {
		return
	}
	delete(m.terminals, t.ID)
	if t.ID == m.selectedID {
		m.focused = false
	}
	m.showMessage("session closed")
}

func (m *model) handleKey(key tea.KeyMsg) tea.Cmd {
	if m.newOpen {
		m.handleNewKey(key)
		return nil
	}
	if m.settingsOpen {
		m.handleSettingsKey(key)
		return nil
	}
	if m.focused {
		m.sendKeyToClaude(key)
		return nil
	}
	if m.filtering {
		m.typeInFilter(key)
		return nil
	}
	return m.handleListKey(key)
}

func (m *model) sendKeyToClaude(key tea.KeyMsg) {
	s := m.selected()
	if key.Type == tea.KeyCtrlQ || s == nil || m.terminals[s.ID] == nil {
		m.focused = false
		return
	}
	if key.Type == tea.KeyCtrlUp {
		m.switchLive(-1)
		return
	}
	if key.Type == tea.KeyCtrlDown {
		m.switchLive(1)
		return
	}
	text := keyToBytes(key)
	if text != "" {
		m.terminals[s.ID].scroll = 0
		m.terminals[s.ID].pty.Write([]byte(text))
	}
}

func (m *model) handleMouse(msg tea.MouseMsg) {
	if m.splash != nil || m.settingsOpen || m.newOpen {
		return
	}
	up := msg.Button == tea.MouseButtonWheelUp
	down := msg.Button == tea.MouseButtonWheelDown
	if msg.Action != tea.MouseActionPress || (!up && !down) {
		return
	}

	if msg.X < m.sidebarWidth() {
		if !m.focused {
			if up {
				m.moveSelection(-1)
			} else {
				m.moveSelection(1)
			}
		}
		return
	}

	s := m.selected()
	if s == nil || m.terminals[s.ID] == nil {
		return
	}
	t := m.terminals[s.ID]

	if t.mouseOn.Load() {
		t.scroll = 0
		wheel := vt.MouseWheel{X: max(0, msg.X-m.sidebarWidth()-1), Y: max(0, msg.Y-2), Button: vt.MouseWheelDown}
		if up {
			wheel.Button = vt.MouseWheelUp
		}
		t.screen.SendMouse(wheel)
	} else if up {
		t.scrollBy(3)
	} else {
		t.scrollBy(-3)
	}
}

func (m *model) typeInFilter(key tea.KeyMsg) {
	switch key.Type {
	case tea.KeyEsc:
		m.filtering = false
		m.filter = ""
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyBackspace:
		letters := []rune(m.filter)
		if len(letters) > 0 {
			m.filter = string(letters[:len(letters)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		m.filter += string(key.Runes)
	case tea.KeyUp:
		m.moveSelection(-1)
	case tea.KeyDown:
		m.moveSelection(1)
	}
}

func (m *model) handleListKey(key tea.KeyMsg) tea.Cmd {
	confirm := m.confirm
	m.confirm = ""

	switch key.String() {
	case "q", "ctrl+c":
		if len(m.terminals) > 0 && confirm != "q" && settingOn("confirmQuit") {
			m.confirm = "q"
			m.showMessage(fmt.Sprintf("%d live session(s) will close · press q again to quit", len(m.terminals)))
			return nil
		}
		return tea.Quit

	case "up", "k":
		m.moveSelection(-1)

	case "down", "j":
		m.moveSelection(1)

	case "tab":
		m.selectNext()

	case "ctrl+up":
		m.switchLive(-1)

	case "ctrl+down":
		m.switchLive(1)

	case "/":
		m.filtering = true

	case ",":
		m.settingsOpen = true

	case "esc":
		m.filter = ""

	case "enter", "right", "l":
		s := m.selected()
		if s != nil {
			m.open(s.Folder, s.ID, true)
		}

	case "n":
		m.openNewSession()

	case "x":
		s := m.selected()
		if s == nil || m.terminals[s.ID] == nil {
			return nil
		}
		if confirm != "x" {
			m.confirm = "x"
			m.showMessage("press x again to close this session")
			return nil
		}
		m.terminals[s.ID].Kill()
	}
	return nil
}

var claudeVariables = []string{
	"CLAUDECODE",
	"CLAUDE_CODE_CHILD_SESSION",
	"CLAUDE_CODE_SESSION_ID",
	"CLAUDE_PID",
	"CLAUDE_CODE_SESSION_ATTENDED",
	"CLAUDE_CODE_ENTRYPOINT",
	"CLAUDE_CODE_EXECPATH",
	"CLAUDE_CODE_MESSAGING_SOCKET",
	"CLAUDE_CODE_MESSAGING_TOKEN",
	"CLAUDE_EFFORT",
	"AI_AGENT",
}

func main() {
	startFolder, _ = os.Getwd()

	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".claude", "projects")

	for _, name := range claudeVariables {
		os.Unsetenv(name)
	}

	loadConfig()
	applyTheme(setting("theme"))

	m := &model{
		root:      root,
		cache:     map[string]*sessions.Session{},
		terminals: map[string]*Terminal{},
	}

	intro := setting("intro")
	if intro == "always" || (intro == "first run" && config["_introSeen"] == "") {
		m.splash = newSplash()
		config["_introSeen"] = "1"
		saveConfig()
	}

	program = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithFPS(120))
	_, err := program.Run()

	for _, t := range m.terminals {
		t.Kill()
	}
	if err != nil {
		fmt.Println("multiclaude crashed:", err)
		os.Exit(1)
	}
}
