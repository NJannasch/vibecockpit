package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLogger_LogCreatesFile(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{path: filepath.Join(dir, "test-audit.jsonl")}

	l.Log("list_sessions", map[string]any{"limit": 5}, []byte(`[{"id":"1"}]`), 1)

	data, err := os.ReadFile(l.path)
	if err != nil {
		t.Fatalf("audit log not created: %v", err)
	}

	var entry Entry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("invalid JSON in audit log: %v", err)
	}

	if entry.Tool != "list_sessions" {
		t.Errorf("Tool = %q, want list_sessions", entry.Tool)
	}
	if entry.ResultCount != 1 {
		t.Errorf("ResultCount = %d, want 1", entry.ResultCount)
	}
	if entry.ResultHash == "" {
		t.Error("ResultHash is empty")
	}
	if entry.Timestamp == "" {
		t.Error("Timestamp is empty")
	}
}

func TestLogger_AppendsMultiple(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{path: filepath.Join(dir, "test-audit.jsonl")}

	l.Log("tool1", nil, []byte("r1"), 1)
	l.Log("tool2", nil, []byte("r2"), 2)

	data, err := os.ReadFile(l.path)
	if err != nil {
		t.Fatal(err)
	}

	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("expected 2 lines, got %d", lines)
	}
}

func TestLogger_Path(t *testing.T) {
	l := NewLogger()
	if l.Path() == "" {
		t.Error("Path() returned empty")
	}
}
