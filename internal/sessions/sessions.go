package sessions

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Status int

const (
	Idle     Status = 0
	Working  Status = 1
	YourTurn Status = 2
	Approval Status = 3
)

type Session struct {
	ID       string
	Path     string
	Folder   string
	Branch   string
	Title    string
	Prompt   string
	Recap    string
	Reply    string
	Tool     string
	Modified time.Time
	Cost     float64
	Added    int
	Removed  int

	LastEvent string

	readUpTo int64
}

func (s *Session) Project() string {
	if s.Folder == "" {
		return filepath.Base(filepath.Dir(s.Path))
	}
	return filepath.Base(s.Folder)
}

func (s *Session) Name() string {
	title := firstLine(s.Title)
	if title != "" {
		return title
	}
	prompt := firstLine(s.Prompt)
	if prompt != "" {
		return prompt
	}
	return "untitled"
}

func firstLine(text string) string {
	lines := strings.Split(text, "\n")
	return strings.TrimSpace(lines[0])
}

func (s *Session) Status(now time.Time) Status {
	age := now.Sub(s.Modified)

	if s.LastEvent == "tool" {
		if age < 20*time.Second {
			return Working
		}
		if age < 6*time.Hour {
			return Approval
		}
	}
	if s.LastEvent == "prompt" || s.LastEvent == "result" {
		if age < 2*time.Minute {
			return Working
		}
	}
	if s.LastEvent == "reply" {
		if age < time.Hour {
			return YourTurn
		}
	}
	return Idle
}

type entry struct {
	Type              string          `json:"type"`
	Subtype           string          `json:"subtype"`
	Cwd               string          `json:"cwd"`
	GitBranch         string          `json:"gitBranch"`
	AiTitle           string          `json:"aiTitle"`
	LastPrompt        string          `json:"lastPrompt"`
	IsSidechain       bool            `json:"isSidechain"`
	IsMeta            bool            `json:"isMeta"`
	Content           json.RawMessage `json:"content"`
	TotalCostUSD      float64         `json:"totalCostUSD"`
	TotalLinesAdded   int             `json:"totalLinesAdded"`
	TotalLinesRemoved int             `json:"totalLinesRemoved"`
	Message           *message        `json:"message"`
}

type message struct {
	StopReason string          `json:"stop_reason"`
	Content    json.RawMessage `json:"content"`
}

type block struct {
	Type  string         `json:"type"`
	Text  string         `json:"text"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func (s *Session) apply(e entry) {
	switch e.Type {
	case "ai-title":
		s.Title = e.AiTitle
	case "last-prompt":
		if e.LastPrompt != "" {
			s.Prompt = e.LastPrompt
		}
	case "cost-state":
		s.Cost = e.TotalCostUSD
		s.Added = e.TotalLinesAdded
		s.Removed = e.TotalLinesRemoved
	case "system":
		if e.Subtype == "away_summary" {
			json.Unmarshal(e.Content, &s.Recap)
		}
	case "user":
		if e.IsSidechain || e.IsMeta || e.Message == nil {
			return
		}
		s.updateFolder(e)
		s.applyUserMessage(e.Message)
	case "assistant":
		if e.IsSidechain || e.IsMeta || e.Message == nil {
			return
		}
		s.updateFolder(e)
		s.applyClaudeMessage(e.Message)
	}
}

func (s *Session) updateFolder(e entry) {
	if e.Cwd == "" {
		return
	}
	s.Folder = e.Cwd
	s.Branch = e.GitBranch
	if s.Branch == "HEAD" {
		s.Branch = ""
	}
}

func (s *Session) applyUserMessage(m *message) {
	text, blocks := decodeContent(m.Content)
	s.LastEvent = "prompt"
	for _, b := range blocks {
		if b.Type == "tool_result" {
			s.LastEvent = "result"
		}
		text += b.Text
	}
	if strings.HasPrefix(text, "[Request interrupted") {
		s.LastEvent = "reply"
	}
}

func (s *Session) applyClaudeMessage(m *message) {
	_, blocks := decodeContent(m.Content)
	usedTool := false
	for _, b := range blocks {
		if b.Type == "text" {
			s.Reply = b.Text
		}
		if b.Type == "tool_use" {
			usedTool = true
			s.Tool = describeTool(b)
		}
	}

	if usedTool || m.StopReason == "tool_use" {
		s.LastEvent = "tool"
	} else if m.StopReason == "end_turn" || m.StopReason == "stop_sequence" || m.StopReason == "refusal" {
		s.LastEvent = "reply"
	} else {
		s.LastEvent = "result"
	}
}

func decodeContent(raw json.RawMessage) (string, []block) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var blocks []block
	json.Unmarshal(raw, &blocks)
	return "", blocks
}

func describeTool(b block) string {
	keys := []string{"command", "description", "file_path", "pattern", "url", "prompt"}
	for _, key := range keys {
		value, isString := b.Input[key].(string)
		if isString && value != "" {
			return b.Name + ": " + firstLine(value)
		}
	}
	return b.Name
}

const maxReadBytes = 4 * 1024 * 1024

func loadSession(path string, modified time.Time) (*Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	s := &Session{ID: id, Path: path, Modified: modified}

	start := int64(0)
	info, err := file.Stat()
	if err == nil && info.Size() > maxReadBytes {
		start = info.Size() - maxReadBytes
	}
	file.Seek(start, io.SeekStart)
	reader := bufio.NewReader(file)

	if start > 0 {
		halfLine, _ := reader.ReadBytes('\n')
		start = start + int64(len(halfLine))
	}

	s.readLines(reader, start)
	return s, nil
}

func readNewLines(old *Session, path string, modified time.Time) (*Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.Size() < old.readUpTo {
		return loadSession(path, modified)
	}

	s := *old
	s.Modified = modified
	file.Seek(old.readUpTo, io.SeekStart)
	s.readLines(bufio.NewReader(file), old.readUpTo)
	return &s, nil
}

func (s *Session) readLines(reader *bufio.Reader, position int64) {
	s.readUpTo = position
	for {
		line, err := reader.ReadBytes('\n')
		var e entry
		if json.Unmarshal(line, &e) == nil {
			s.apply(e)
		}
		if err != nil {
			return
		}
		position = position + int64(len(line))
		s.readUpTo = position
	}
}

type fileToRead struct {
	path     string
	old      *Session
	modified time.Time
	result   *Session
}

func (f *fileToRead) read() {
	var s *Session
	var err error
	if f.old == nil {
		s, err = loadSession(f.path, f.modified)
	} else {
		s, err = readNewLines(f.old, f.path, f.modified)
	}
	if err == nil {
		f.result = s
	}
}

func readAll(files []*fileToRead) {
	var wg sync.WaitGroup
	freeWorkers := make(chan bool, runtime.NumCPU())
	for _, f := range files {
		wg.Add(1)
		freeWorkers <- true
		go func(f *fileToRead) {
			f.read()
			<-freeWorkers
			wg.Done()
		}(f)
	}
	wg.Wait()
}

func Scan(root string, cache map[string]*Session) []*Session {
	paths, _ := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))

	var files []*fileToRead
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		old := cache[path]
		if old != nil && old.Modified.Equal(info.ModTime()) {
			continue
		}
		files = append(files, &fileToRead{path: path, old: old, modified: info.ModTime()})
	}

	readAll(files)
	for _, f := range files {
		if f.result != nil {
			cache[f.path] = f.result
		}
	}

	var sessions []*Session
	for _, path := range paths {
		s := cache[path]
		if s != nil && s.LastEvent != "" {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

func Sort(sessions []*Session, now time.Time) {
	sort.SliceStable(sessions, func(i, j int) bool {
		a := sessions[i].Status(now)
		b := sessions[j].Status(now)
		if a != b {
			return a > b
		}
		return sessions[i].Modified.After(sessions[j].Modified)
	})
}
