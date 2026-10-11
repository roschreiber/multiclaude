package sessions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func appendTo(t *testing.T, path, text string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(text)
}

func TestScanReadsNewLinesOnly(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "proj"), 0o755)
	path := filepath.Join(root, "proj", "abc.jsonl")
	cache := map[string]*Session{}

	appendTo(t, path, `{"type":"user","cwd":"/work","message":{"content":"hi"}}`+"\n")
	list := Scan(root, cache)
	if len(list) != 1 || list[0].LastEvent != "prompt" || list[0].Folder != "/work" {
		t.Fatalf("first scan: %+v", list)
	}

	appendTo(t, path, `{"type":"assistant","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}`)
	future := time.Now().Add(time.Minute)
	os.Chtimes(path, future, future)
	list = Scan(root, cache)
	if list[0].LastEvent != "prompt" {
		t.Fatalf("half line was read: %+v", list[0])
	}

	appendTo(t, path, "}\n")
	future = future.Add(time.Minute)
	os.Chtimes(path, future, future)
	list = Scan(root, cache)
	if list[0].LastEvent != "reply" || list[0].Reply != "done" {
		t.Fatalf("second scan: %+v", list[0])
	}
}

func TestStatus(t *testing.T) {
	now := time.Now()
	s := &Session{LastEvent: "tool", Modified: now.Add(-time.Minute)}
	if s.Status(now) != Approval {
		t.Error("old tool call should need approval")
	}
	s.Modified = now.Add(-time.Second)
	if s.Status(now) != Working {
		t.Error("fresh tool call should be working")
	}
}
