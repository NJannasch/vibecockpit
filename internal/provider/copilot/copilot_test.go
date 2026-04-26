package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanEvents_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	events := []string{
		mustJSON(t, event{Type: "session.model_change", Data: mustRaw(t, map[string]string{"newModel": "gpt-4o"})}),
		mustJSON(t, event{Type: "user.message", Data: mustRaw(t, map[string]string{"content": "hello world"})}),
		mustJSON(t, event{Type: "assistant.turn_start", Data: mustRaw(t, map[string]string{})}),
		mustJSON(t, event{Type: "user.message", Data: mustRaw(t, map[string]string{"content": "second prompt"})}),
		mustJSON(t, event{Type: "assistant.turn_start", Data: mustRaw(t, map[string]string{})}),
	}
	if err := os.WriteFile(path, []byte(strings.Join(events, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	model, firstPrompt, msgCount := scanEvents(path)

	if model != "gpt-4o" {
		t.Errorf("model: got %q, want gpt-4o", model)
	}
	if firstPrompt != "hello world" {
		t.Errorf("firstPrompt: got %q, want 'hello world'", firstPrompt)
	}
	// 2 user.message + 2 assistant.turn_start = 4
	if msgCount != 4 {
		t.Errorf("msgCount: got %d, want 4", msgCount)
	}
}

func TestScanEvents_MultipleModelChanges_LastWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	events := []string{
		mustJSON(t, event{Type: "session.model_change", Data: mustRaw(t, map[string]string{"newModel": "gpt-4o"})}),
		mustJSON(t, event{Type: "user.message", Data: mustRaw(t, map[string]string{"content": "first"})}),
		mustJSON(t, event{Type: "session.model_change", Data: mustRaw(t, map[string]string{"newModel": "claude-sonnet-4"})}),
		mustJSON(t, event{Type: "session.model_change", Data: mustRaw(t, map[string]string{"newModel": "o3-pro"})}),
	}
	if err := os.WriteFile(path, []byte(strings.Join(events, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	model, firstPrompt, msgCount := scanEvents(path)

	if model != "o3-pro" {
		t.Errorf("model: got %q, want o3-pro", model)
	}
	if firstPrompt != "first" {
		t.Errorf("firstPrompt: got %q, want first", firstPrompt)
	}
	if msgCount != 1 {
		t.Errorf("msgCount: got %d, want 1", msgCount)
	}
}

func TestScanSession(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "session-abc")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	wsYAML := `id: abc-123
cwd: /home/user/project
summary: test session summary
created_at: "2025-06-01T10:00:00Z"
updated_at: "2025-06-01T12:00:00Z"
`
	if err := os.WriteFile(filepath.Join(sessDir, "workspace.yaml"), []byte(wsYAML), 0644); err != nil {
		t.Fatal(err)
	}

	events := []string{
		mustJSON(t, event{Type: "session.model_change", Data: mustRaw(t, map[string]string{"newModel": "gpt-4o"})}),
		mustJSON(t, event{Type: "user.message", Data: mustRaw(t, map[string]string{"content": "build me a CLI"})}),
		mustJSON(t, event{Type: "assistant.turn_start", Data: mustRaw(t, map[string]string{})}),
	}
	if err := os.WriteFile(filepath.Join(sessDir, "events.jsonl"), []byte(strings.Join(events, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	c := &Copilot{baseDir: dir}
	s, err := c.scanSession(sessDir)
	if err != nil {
		t.Fatal(err)
	}

	if s.ID != "abc-123" {
		t.Errorf("ID: got %q, want abc-123", s.ID)
	}
	if s.Provider != "copilot" {
		t.Errorf("Provider: got %q, want copilot", s.Provider)
	}
	if s.ProjectPath != "/home/user/project" {
		t.Errorf("ProjectPath: got %q, want /home/user/project", s.ProjectPath)
	}
	if s.ProjectName != "project" {
		t.Errorf("ProjectName: got %q, want project", s.ProjectName)
	}
	if s.Summary != "test session summary" {
		t.Errorf("Summary: got %q, want 'test session summary'", s.Summary)
	}
	if s.FirstPrompt != "build me a CLI" {
		t.Errorf("FirstPrompt: got %q, want 'build me a CLI'", s.FirstPrompt)
	}
	if s.Model != "gpt-4o" {
		t.Errorf("Model: got %q, want gpt-4o", s.Model)
	}
	if s.MessageCount != 2 {
		t.Errorf("MessageCount: got %d, want 2", s.MessageCount)
	}
}

func TestScanEvents_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	model, firstPrompt, msgCount := scanEvents(path)
	if model != "" {
		t.Errorf("model: got %q, want empty", model)
	}
	if firstPrompt != "" {
		t.Errorf("firstPrompt: got %q, want empty", firstPrompt)
	}
	if msgCount != 0 {
		t.Errorf("msgCount: got %d, want 0", msgCount)
	}
}

// mustJSON marshals v to a JSON string, failing the test on error.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// mustRaw marshals v to json.RawMessage, failing the test on error.
func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return json.RawMessage(data)
}
