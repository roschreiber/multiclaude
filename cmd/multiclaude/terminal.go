package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aymanbagabas/go-pty"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"

	"github.com/roschreiber/multiclaude/internal/devcontainer"
)

type Terminal struct {
	ID        string
	Folder    string
	Started   time.Time
	Container *devcontainer.Container
	pty       pty.Pty
	cmd       *pty.Cmd
	screen    *vt.SafeEmulator

	lock    sync.Mutex
	visible bool
	waiting bool
	changed bool

	afterEscape bool
	insideOSC   bool

	history sync.RWMutex
	mouseOn atomic.Bool
	scroll  int
}

type outputMsg struct{ terminal *Terminal }
type exitedMsg struct{ terminal *Terminal }

var programPaths = map[string]string{}

func findProgram(name string) (string, error) {
	path, found := programPaths[name]
	if found {
		return path, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	programPaths[name] = path
	return path, nil
}

func startTerminal(container *devcontainer.Container, folder, id string, resume bool, width, height int) (*Terminal, error) {
	flag := "--session-id"
	if resume {
		flag = "--resume"
	}

	var command string
	var args []string
	dir := ""
	if container == nil {
		claudePath, err := findProgram("claude")
		if err != nil {
			return nil, err
		}
		command = claudePath
		args = []string{flag, id, "--settings", CCPath()}
		dir = folder
	} else {
		dockerPath, err := findProgram("docker")
		if err != nil {
			return nil, err
		}
		command = dockerPath
		args = []string{"exec", "-it", "-e", "TERM=xterm-256color", "-w", folder}
		if container.User != "" {
			args = append(args, "-u", container.User)
		}
		args = append(args, container.ID, "claude", flag, id)
	}

	p, err := pty.New()
	if err != nil {
		return nil, err
	}
	err = p.Resize(width, height)
	if err != nil {
		p.Close()
		return nil, err
	}

	cmd := p.Command(command, args...)
	cmd.Dir = dir
	err = cmd.Start()
	if err != nil {
		p.Close()
		return nil, err
	}

	screen := vt.NewSafeEmulator(width, height)
	screen.SetScrollbackSize(2000)

	t := &Terminal{
		ID:        id,
		Folder:    folder,
		Started:   time.Now(),
		Container: container,
		pty:       p,
		cmd:       cmd,
		screen:    screen,
	}

	screen.SetCallbacks(vt.Callbacks{
		EnableMode:  func(mode ansi.Mode) { t.trackMouse(mode, true) },
		DisableMode: func(mode ansi.Mode) { t.trackMouse(mode, false) },
	})

	go io.Copy(p, t.screen)
	go t.readOutput()
	go t.waitForExit()
	return t, nil
}

func (t *Terminal) readOutput() {
	buffer := make([]byte, 32*1024)
	for {
		n, err := t.pty.Read(buffer)
		if n > 0 {
			t.history.Lock()
			t.screen.Write(t.removeUnicodeFromOSC(buffer[:n]))
			t.history.Unlock()

			t.lock.Lock()
			t.changed = true
			sendNow := t.visible && !t.waiting
			if sendNow {
				t.waiting = true
			}
			t.lock.Unlock()

			if sendNow {
				program.Send(outputMsg{terminal: t})
			}
		}
		if err != nil {
			return
		}
	}
}

func (t *Terminal) removeUnicodeFromOSC(data []byte) []byte {
	clean := data[:0]
	for _, b := range data {
		if t.insideOSC && (b == 7 || b == 27) {
			t.insideOSC = false
		}
		if t.afterEscape && b == ']' {
			t.insideOSC = true
		}
		t.afterEscape = b == 27
		if t.insideOSC && b >= 128 {
			continue
		}
		clean = append(clean, b)
	}
	return clean
}

func (t *Terminal) redrawDone() tea.Cmd {
	t.lock.Lock()
	defer t.lock.Unlock()

	if !t.changed {
		t.waiting = false
		return nil
	}

	t.changed = false
	return tea.Tick(8*time.Millisecond, func(time.Time) tea.Msg {
		return outputMsg{terminal: t}
	})
}

func (t *Terminal) trackMouse(mode ansi.Mode, on bool) {
	switch mode {
	case ansi.ModeMouseX10, ansi.ModeMouseNormal, ansi.ModeMouseHighlight, ansi.ModeMouseButtonEvent, ansi.ModeMouseAnyEvent:
		t.mouseOn.Store(on)
	}
}

func (t *Terminal) scrollBy(lines int) {
	t.history.RLock()
	defer t.history.RUnlock()

	if t.screen.IsAltScreen() {
		t.scroll = 0
		return
	}
	t.scroll = max(0, min(t.scroll+lines, t.screen.ScrollbackLen()))
}

func (t *Terminal) visibleLines(height int) []string {
	screenLines := strings.Split(t.screen.Render(), "\n")

	t.history.RLock()
	defer t.history.RUnlock()

	if t.scroll == 0 || t.screen.IsAltScreen() {
		return screenLines
	}

	history := t.screen.Scrollback()
	end := history.Len() + len(screenLines) - t.scroll
	var lines []string
	for i := max(0, end-height); i < end; i++ {
		if i < history.Len() {
			lines = append(lines, history.Line(i).Render())
		} else {
			lines = append(lines, screenLines[i-history.Len()])
		}
	}
	return lines
}

func (t *Terminal) setVisible(visible bool) {
	t.lock.Lock()
	t.visible = visible
	t.lock.Unlock()
}

func (t *Terminal) waitForExit() {
	t.cmd.Wait()
	t.pty.Close()
	t.screen.Close()
	program.Send(exitedMsg{terminal: t})
}

func (t *Terminal) Resize(width, height int) {
	t.pty.Resize(width, height)
	t.screen.Resize(width, height)
}

func (t *Terminal) Kill() {
	if t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}
	if t.Container != nil {
		devcontainer.RunDocker("exec", t.Container.ID, "pkill", "-f", t.ID)
	}
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func keyToBytes(key tea.KeyMsg) string {
	text := ""
	if key.Type == tea.KeyRunes && key.Paste {
		text = "\x1b[200~" + string(key.Runes) + "\x1b[201~"
	} else if key.Type == tea.KeyRunes {
		text = string(key.Runes)
	} else if key.Type >= 0 && key.Type <= 31 || key.Type == 127 {
		text = string(rune(key.Type))
	} else {
		text = specialKeys[key.Type]
	}

	if key.Alt && text != "" {
		text = "\x1b" + text
	}
	return text
}

var specialKeys = map[tea.KeyType]string{
	tea.KeyUp:       "\x1b[A",
	tea.KeyDown:     "\x1b[B",
	tea.KeyRight:    "\x1b[C",
	tea.KeyLeft:     "\x1b[D",
	tea.KeyShiftTab: "\x1b[Z",
	tea.KeyHome:     "\x1b[H",
	tea.KeyEnd:      "\x1b[F",
	tea.KeyPgUp:     "\x1b[5~",
	tea.KeyPgDown:   "\x1b[6~",
	tea.KeyDelete:   "\x1b[3~",
	tea.KeyInsert:   "\x1b[2~",
	tea.KeySpace:    " ",

	tea.KeyCtrlLeft:  "\x1b[1;5D",
	tea.KeyCtrlRight: "\x1b[1;5C",
}
