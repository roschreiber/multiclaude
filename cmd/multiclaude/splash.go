package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const logo = `███╗   ███╗██╗   ██╗██╗     ████████╗██╗
████╗ ████║██║   ██║██║     ╚══██╔══╝██║
██╔████╔██║██║   ██║██║        ██║   ██║
██║╚██╔╝██║██║   ██║██║        ██║   ██║
██║ ╚═╝ ██║╚██████╔╝███████╗   ██║   ██║
╚═╝     ╚═╝ ╚═════╝ ╚══════╝   ╚═╝   ╚═╝
 ██████╗██╗      █████╗ ██╗   ██╗██████╗ ███████╗
██╔════╝██║     ██╔══██╗██║   ██║██╔══██╗██╔════╝
██║     ██║     ███████║██║   ██║██║  ██║█████╗
██║     ██║     ██╔══██║██║   ██║██║  ██║██╔══╝
╚██████╗███████╗██║  ██║╚██████╔╝██████╔╝███████╗
 ╚═════╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝`

const (
	eyesForward = "  ██▀███▀█  "
	eyesBack    = "  █▀███▀██  "
	clawdArms   = "████████████"
	clawdBelly  = "  ████████  "
	legsStand   = "  █ █  █ █  "
	legsStep1   = "  ▀ █  ▀ █  "
	legsStep2   = "  █ ▀  █ ▀  "
	clawdWidth  = 12
)

const stageHeight = 6

type clawd struct {
	x     int
	speed int
}

type splash struct {
	frame     int
	phase     string
	waitUntil int
	clawds    []clawd
	logo      string
}

type splashTickMsg struct{}

func splashTick() tea.Cmd {
	return tea.Tick(40*time.Millisecond, func(time.Time) tea.Msg {
		return splashTickMsg{}
	})
}

func newSplash() *splash {
	return &splash{
		phase:  "alone",
		clawds: []clawd{{x: -clawdWidth, speed: 2}},
		logo:   colorLogo(logo),
	}
}

func colorLogo(text string) string {
	block := colored(orange)
	shadow := colored(logoShadow)
	var out strings.Builder
	for _, r := range text {
		if r == '█' {
			out.WriteString(block.Render("█"))
		} else if r == ' ' || r == '\n' {
			out.WriteRune(r)
		} else {
			out.WriteString(shadow.Render(string(r)))
		}
	}
	return out.String()
}

func (s *splash) step(width int) {
	s.frame++
	leader := &s.clawds[0]

	if s.phase == "alone" {
		if leader.x >= width/2-clawdWidth/2 {
			s.phase = "waiting"
			leader.speed = 0
			s.waitUntil = s.frame + 25
		}
	} else if s.phase == "waiting" {
		if s.frame >= s.waitUntil {
			s.phase = "gang"
			for i := 1; i <= 6; i++ {
				s.clawds = append(s.clawds, clawd{x: -14 * i, speed: 3})
			}
		}
	} else if s.phase == "gang" {
		if s.clawds[1].x > leader.x-24 {
			s.phase = "leaving"
			leader.speed = 3
		}
	}

	for i := range s.clawds {
		s.clawds[i].x += s.clawds[i].speed
	}
}

func (s *splash) done(width int) bool {
	last := s.clawds[len(s.clawds)-1]
	return s.phase == "leaving" && last.x > width
}

func drawText(row []rune, x int, text string) {
	for i, letter := range []rune(text) {
		position := x + i
		if letter != ' ' && position >= 0 && position < len(row) {
			row[position] = letter
		}
	}
}

func (s *splash) drawClawds(stage [][]rune) {
	for i, c := range s.clawds {
		step := (s.frame/2 + i) % 2
		moving := c.speed > 0

		eyes := eyesForward
		legs := legsStand
		top := 2
		if moving && step == 0 {
			legs = legsStep1
		}
		if moving && step == 1 {
			legs = legsStep2
		}
		if moving && (s.frame/3+i)%2 == 0 {
			top = 1
		}
		if i == 0 && s.phase == "waiting" && (s.frame/8)%2 == 0 {
			eyes = eyesBack
		}

		drawText(stage[top], c.x, eyes)
		drawText(stage[top+1], c.x, clawdArms)
		drawText(stage[top+2], c.x, clawdBelly)
		drawText(stage[top+3], c.x, legs)
	}
}

func (m *model) splashView() string {
	s := m.splash

	stage := make([][]rune, stageHeight)
	for y := range stage {
		stage[y] = []rune(strings.Repeat(" ", m.width))
	}
	s.drawClawds(stage)

	bubble := ""
	if s.phase == "waiting" {
		bubble = "?"
	}
	if s.phase == "gang" || s.phase == "leaving" {
		bubble = "!"
	}
	drawText(stage[0], s.clawds[0].x+clawdWidth/2, bubble)

	var lines []string

	logoWidth := lipgloss.Width(s.logo)
	indent := strings.Repeat(" ", max(0, (m.width-logoWidth)/2))
	for _, line := range strings.Split(s.logo, "\n") {
		lines = append(lines, indent+line)
	}
	lines = append(lines, "")

	lines = append(lines, colored(cream).Render(string(stage[0])))
	for y := 1; y < stageHeight; y++ {
		lines = append(lines, colored(orange).Render(string(stage[y])))
	}

	prompt := colored(muted).Render("press any key")
	lines = append(lines, "", lipgloss.PlaceHorizontal(m.width, lipgloss.Center, prompt))

	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "")
	}
	return lipgloss.PlaceVertical(m.height, lipgloss.Center, strings.Join(lines, "\n"))
}
