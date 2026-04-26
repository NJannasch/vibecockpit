package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractModelFromTail_Normal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"user","message":{"content":"hello"}}`,
		`{"type":"assistant","message":{"model":"claude-sonnet-4-20250514","content":"hi"}}`,
		`{"type":"user","message":{"content":"thanks"}}`,
		`{"type":"assistant","message":{"model":"claude-sonnet-4-20250514","content":"bye"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := extractModelFromTail(path)
	if got != "claude-sonnet-4-20250514" {
		t.Errorf("expected claude-sonnet-4-20250514, got %q", got)
	}
}

func TestExtractModelFromTail_NoAssistantMessages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"user","message":{"content":"hello"}}`,
		`{"type":"user","message":{"content":"another"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := extractModelFromTail(path)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractModelFromTail_NonClaudeModelFiltered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"user","message":{"content":"hello"}}`,
		`{"type":"assistant","message":{"model":"gpt-4","content":"hi"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := extractModelFromTail(path)
	if got != "" {
		t.Errorf("expected empty string for non-claude model, got %q", got)
	}
}

func TestExtractModelFromTail_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	// Create a file larger than 32KB so the seek logic is exercised.
	// Fill with padding user messages, then put an assistant message at the end.
	var sb strings.Builder
	padding := `{"type":"user","message":{"content":"` + strings.Repeat("x", 500) + `"}}`
	for sb.Len() < 40000 {
		sb.WriteString(padding)
		sb.WriteByte('\n')
	}
	sb.WriteString(`{"type":"assistant","message":{"model":"claude-opus-4-20250514","content":"done"}}`)
	sb.WriteByte('\n')

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}

	fi, _ := os.Stat(path)
	if fi.Size() <= 32768 {
		t.Fatalf("test file too small: %d bytes, need >32768", fi.Size())
	}

	got := extractModelFromTail(path)
	if got != "claude-opus-4-20250514" {
		t.Errorf("expected claude-opus-4-20250514, got %q", got)
	}
}

func TestExtractMetadataFromJSONL_StringContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	ts := "2025-03-15T10:30:00Z"
	line := `{"type":"user","cwd":"/home/user/project","gitBranch":"main","timestamp":"` + ts + `","message":{"content":"build the app"}}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	meta := extractMetadataFromJSONL(path)

	if meta.cwd != "/home/user/project" {
		t.Errorf("cwd: got %q, want /home/user/project", meta.cwd)
	}
	if meta.gitBranch != "main" {
		t.Errorf("gitBranch: got %q, want main", meta.gitBranch)
	}
	if meta.firstPrompt != "build the app" {
		t.Errorf("firstPrompt: got %q, want 'build the app'", meta.firstPrompt)
	}
	expected, _ := time.Parse(time.RFC3339, ts)
	if !meta.created.Equal(expected) {
		t.Errorf("created: got %v, want %v", meta.created, expected)
	}
}

func TestExtractMetadataFromJSONL_ArrayContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	content := `[{"type":"text","text":"hello "},{"type":"text","text":"world"}]`
	line := `{"type":"user","cwd":"/tmp/test","gitBranch":"dev","timestamp":"2025-01-01T00:00:00Z","message":{"content":` + content + `}}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	meta := extractMetadataFromJSONL(path)

	if meta.cwd != "/tmp/test" {
		t.Errorf("cwd: got %q, want /tmp/test", meta.cwd)
	}
	if meta.gitBranch != "dev" {
		t.Errorf("gitBranch: got %q, want dev", meta.gitBranch)
	}
	if meta.firstPrompt != "hello world" {
		t.Errorf("firstPrompt: got %q, want 'hello world'", meta.firstPrompt)
	}
}

func TestDecodeProjectDir(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"-home-user-project", "/home/user/project"},
		{"normaldir", "normaldir"},
		{"-usr-local-bin", "/usr/local/bin"},
	}

	for _, tt := range tests {
		got := decodeProjectDir(tt.input)
		if got != tt.want {
			t.Errorf("decodeProjectDir(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestScanProject_WithSessionsIndex(t *testing.T) {
	dir := t.TempDir()
	projPath := filepath.Join(dir, "-home-user-myproject")
	if err := os.MkdirAll(projPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a JSONL file for the session
	sessionID := "abc-123"
	jsonlPath := filepath.Join(projPath, sessionID+".jsonl")
	jsonlLines := []string{
		`{"type":"user","message":{"content":"init"}}`,
		`{"type":"assistant","message":{"model":"claude-sonnet-4-20250514","content":"ok"}}`,
	}
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(jsonlLines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create sessions-index.json with one normal entry and one sidechain
	idx := indexFile{
		Version:      1,
		OriginalPath: "/home/user/myproject",
		Entries: []indexEntry{
			{
				SessionID:    sessionID,
				FullPath:     jsonlPath,
				FirstPrompt:  "init",
				Summary:      "test session",
				MessageCount: 2,
				Created:      "2025-03-15T10:00:00Z",
				Modified:     "2025-03-15T11:00:00Z",
				GitBranch:    "feature-x",
				ProjectPath:  "/home/user/myproject",
				IsSidechain:  false,
			},
			{
				SessionID:   "sidechain-456",
				FullPath:    filepath.Join(projPath, "sidechain-456.jsonl"),
				IsSidechain: true,
			},
		},
	}
	idxData, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projPath, "sessions-index.json"), idxData, 0644); err != nil {
		t.Fatal(err)
	}

	active := map[string]int{}
	sessions := scanProject(projPath, "-home-user-myproject", active)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ID != sessionID {
		t.Errorf("ID: got %q, want %q", s.ID, sessionID)
	}
	if s.Provider != "claude" {
		t.Errorf("Provider: got %q, want claude", s.Provider)
	}
	if s.ProjectPath != "/home/user/myproject" {
		t.Errorf("ProjectPath: got %q, want /home/user/myproject", s.ProjectPath)
	}
	if s.ProjectName != "myproject" {
		t.Errorf("ProjectName: got %q, want myproject", s.ProjectName)
	}
	if s.Summary != "test session" {
		t.Errorf("Summary: got %q, want 'test session'", s.Summary)
	}
	if s.FirstPrompt != "init" {
		t.Errorf("FirstPrompt: got %q, want init", s.FirstPrompt)
	}
	if s.GitBranch != "feature-x" {
		t.Errorf("GitBranch: got %q, want feature-x", s.GitBranch)
	}
	if s.MessageCount != 2 {
		t.Errorf("MessageCount: got %d, want 2", s.MessageCount)
	}
	if s.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model: got %q, want claude-sonnet-4-20250514", s.Model)
	}
}
