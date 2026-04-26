package gemini

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chat1.json")

	sf := sessionFile{
		SessionID:   "sess-001",
		ProjectHash: "abc123",
		StartTime:   "2025-05-10T08:00:00Z",
		LastUpdated: "2025-05-10T09:30:00Z",
		Messages: []message{
			{Type: "user", Content: "explain concurrency in Go"},
			{Type: "gemini", Content: "Concurrency in Go uses goroutines..."},
			{Type: "user", Content: "show me an example"},
			{Type: "gemini", Content: "Here is an example..."},
		},
	}
	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	g := &Gemini{baseDir: dir}
	s, err := g.parseSession(path, "abc123", "/home/user/goproject")
	if err != nil {
		t.Fatal(err)
	}

	if s.ID != "sess-001" {
		t.Errorf("ID: got %q, want sess-001", s.ID)
	}
	if s.Provider != "gemini" {
		t.Errorf("Provider: got %q, want gemini", s.Provider)
	}
	if s.ProjectName != "goproject" {
		t.Errorf("ProjectName: got %q, want goproject", s.ProjectName)
	}
	if s.ProjectPath != "/home/user/goproject" {
		t.Errorf("ProjectPath: got %q, want /home/user/goproject", s.ProjectPath)
	}
	if s.FirstPrompt != "explain concurrency in Go" {
		t.Errorf("FirstPrompt: got %q, want 'explain concurrency in Go'", s.FirstPrompt)
	}
	if s.MessageCount != 4 {
		t.Errorf("MessageCount: got %d, want 4", s.MessageCount)
	}
	if s.Model != "gemini" {
		t.Errorf("Model: got %q, want gemini", s.Model)
	}
	if s.Created.IsZero() {
		t.Error("Created should not be zero")
	}
	if s.Modified.IsZero() {
		t.Error("Modified should not be zero")
	}
}

func TestParseSession_FallbackProjectName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chat.json")

	sf := sessionFile{
		SessionID:   "sess-002",
		StartTime:   "2025-01-01T00:00:00Z",
		LastUpdated: "2025-01-01T01:00:00Z",
		Messages:    []message{{Type: "user", Content: "hi"}},
	}
	data, _ := json.Marshal(sf)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	g := &Gemini{baseDir: dir}
	// Pass empty projectPath -- dirName should be used as ProjectName
	s, err := g.parseSession(path, "myhash", "")
	if err != nil {
		t.Fatal(err)
	}

	if s.ProjectName != "gemini-session" {
		t.Errorf("ProjectName: got %q, want gemini-session (fallback for unresolved hash)", s.ProjectName)
	}
}

func TestTruncateFirstLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short single line",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "multi-line truncated to first",
			input: "first line\nsecond line\nthird line",
			want:  "first line",
		},
		{
			name:  "long line gets ellipsis",
			input: "This is a very long line that exceeds eighty characters and should be truncated with an ellipsis at the end of it.",
			want:  "This is a very long line that exceeds eighty characters and should be truncat...",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "exactly 80 chars unchanged",
			input: "12345678901234567890123456789012345678901234567890123456789012345678901234567890",
			want:  "12345678901234567890123456789012345678901234567890123456789012345678901234567890",
		},
		{
			name:  "81 chars truncated",
			input: "123456789012345678901234567890123456789012345678901234567890123456789012345678901",
			want:  "12345678901234567890123456789012345678901234567890123456789012345678901234567...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateFirstLine(tt.input)
			if got != tt.want {
				t.Errorf("truncateFirstLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoadProjects(t *testing.T) {
	dir := t.TempDir()
	projectsJSON := `{"projects":{"/home/user/project-a":"hash-a","/home/user/project-b":"hash-b"}}`
	if err := os.WriteFile(filepath.Join(dir, "projects.json"), []byte(projectsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	g := &Gemini{baseDir: dir}
	projects := g.loadProjects()

	if projects == nil {
		t.Fatal("loadProjects returned nil")
	}
	if projects["hash-a"] != "/home/user/project-a" {
		t.Errorf("hash-a: got %q, want /home/user/project-a", projects["hash-a"])
	}
	if projects["hash-b"] != "/home/user/project-b" {
		t.Errorf("hash-b: got %q, want /home/user/project-b", projects["hash-b"])
	}
}

func TestLoadProjects_MissingFile(t *testing.T) {
	dir := t.TempDir()
	g := &Gemini{baseDir: dir}
	projects := g.loadProjects()
	if projects != nil {
		t.Errorf("expected nil for missing file, got %v", projects)
	}
}

func TestResolveProjectRoot(t *testing.T) {
	dir := t.TempDir()
	historyDir := filepath.Join(dir, "history", "somehash")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(historyDir, ".project_root"), []byte("/home/user/my-project\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := resolveProjectRoot(historyDir)
	if got != "/home/user/my-project" {
		t.Errorf("resolveProjectRoot: got %q, want /home/user/my-project", got)
	}
}

func TestResolveProjectRoot_Missing(t *testing.T) {
	dir := t.TempDir()
	got := resolveProjectRoot(dir)
	if got != "" {
		t.Errorf("expected empty for missing .project_root, got %q", got)
	}
}
