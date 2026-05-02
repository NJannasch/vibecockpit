package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vibecockpit/internal/board"
)

func TestComposePrompt(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("test"), 0644)

	task := &board.Task{
		ID:          "test-task",
		Title:       "Fix the bug",
		Description: "The login form crashes on submit",
		Acceptance:  []string{"run: make test", "Login works"},
	}

	prompt := composePrompt(task, dir, "")

	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should mention CLAUDE.md")
	}
	if !strings.Contains(prompt, "AGENTS.md") {
		t.Error("prompt should mention AGENTS.md")
	}
	if !strings.Contains(prompt, "STATUS.md") {
		t.Error("prompt should mention STATUS.md")
	}
	if !strings.Contains(prompt, "Fix the bug") {
		t.Error("prompt should contain task title")
	}
	if !strings.Contains(prompt, "login form crashes") {
		t.Error("prompt should contain description")
	}
	if !strings.Contains(prompt, "make test") {
		t.Error("prompt should contain acceptance criteria")
	}
}

func TestComposePromptNoInstructionFiles(t *testing.T) {
	dir := t.TempDir()
	task := &board.Task{ID: "test", Title: "Do something"}

	prompt := composePrompt(task, dir, "")

	if strings.Contains(prompt, "Read ") && strings.Contains(prompt, " first") {
		t.Error("prompt should not mention instruction files when none exist")
	}
	if !strings.Contains(prompt, "Do something") {
		t.Error("prompt should contain title")
	}
}

func TestToolConfigFor(t *testing.T) {
	tests := []struct {
		tool    string
		wantBin string
		wantMCP string
	}{
		{"claude", "claude", ".mcp.json"},
		{"codex", "codex", "codex.json"},
		{"gemini", "gemini", ".gemini/settings.json"},
		{"opencode", "opencode", "opencode.json"},
		{"unknown", "unknown", ".mcp.json"},
	}
	for _, tt := range tests {
		tc := toolConfigFor(tt.tool)
		if tc.bin != tt.wantBin {
			t.Errorf("toolConfigFor(%q).bin = %q, want %q", tt.tool, tc.bin, tt.wantBin)
		}
		if tc.mcpFile != tt.wantMCP {
			t.Errorf("toolConfigFor(%q).mcpFile = %q, want %q", tt.tool, tc.mcpFile, tt.wantMCP)
		}
	}
}

func TestBuildArgs(t *testing.T) {
	tc := toolConfigFor("claude")
	args := buildArgs(tc, "sonnet-4.6", "fix the bug")

	found := false
	for _, a := range args {
		if a == "fix the bug" {
			found = true
		}
	}
	if !found {
		t.Error("args should contain prompt")
	}

	hasModel := false
	for i, a := range args {
		if a == "--model" && i+1 < len(args) && args[i+1] == "sonnet-4.6" {
			hasModel = true
		}
	}
	if !hasModel {
		t.Error("args should contain --model sonnet-4.6")
	}
}

func TestEnsureMCPConfig(t *testing.T) {
	dir := t.TempDir()
	tc := toolConfigFor("claude")

	if err := ensureMCPConfig(dir, tc, nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), "vibecockpit") {
		t.Error("MCP config should contain vibecockpit server")
	}
	if !strings.Contains(string(data), "--mcp") {
		t.Error("MCP config should contain --mcp arg")
	}
}

func TestEnsureMCPConfigPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	existing := `{"mcpServers":{"postgres":{"command":"pg-mcp"}}}`
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(existing), 0644)

	tc := toolConfigFor("claude")
	if err := ensureMCPConfig(dir, tc, nil); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	content := string(data)
	if !strings.Contains(content, "postgres") {
		t.Error("should preserve existing MCP servers")
	}
	if !strings.Contains(content, "vibecockpit") {
		t.Error("should add vibecockpit")
	}
}

func TestCheckAcceptanceCriteria(t *testing.T) {
	dir := t.TempDir()
	task := &board.Task{
		Acceptance: []string{
			"run: echo hello",
			"run: false",
			"Manual check",
		},
	}

	failed, output := checkAcceptanceCriteria(task, dir)

	if len(failed) != 1 {
		t.Errorf("expected 1 failure, got %d", len(failed))
	}
	if !strings.Contains(output, "PASS: echo hello") {
		t.Error("should show passing criteria")
	}
	if !strings.Contains(output, "FAIL: false") {
		t.Error("should show failing criteria")
	}
}

func TestCheckAcceptanceNoRunCriteria(t *testing.T) {
	task := &board.Task{
		Acceptance: []string{"Manual check only", "Another manual"},
	}

	failed, _ := checkAcceptanceCriteria(task, t.TempDir())
	if len(failed) != 0 {
		t.Error("manual criteria should not be checked")
	}
}

func TestDefaultAllowedTools(t *testing.T) {
	tools := defaultAllowedTools()
	if len(tools) == 0 {
		t.Error("should return tools")
	}

	hasRead := false
	hasMCP := false
	for _, tool := range tools {
		if tool == "Read" {
			hasRead = true
		}
		if tool == "mcp__vibecockpit" {
			hasMCP = true
		}
	}
	if !hasRead {
		t.Error("should include Read")
	}
	if !hasMCP {
		t.Error("should include mcp__vibecockpit")
	}
}

func TestIsGitRepo(t *testing.T) {
	if isGitRepo(t.TempDir()) {
		t.Error("temp dir should not be a git repo")
	}
}

func TestTracker(t *testing.T) {
	trackerMu.Lock()
	oldPath := persistPath
	persistPath = filepath.Join(t.TempDir(), "test-agents.json")
	activeRuns = make(map[string]*AgentRun)
	trackerMu.Unlock()
	defer func() {
		trackerMu.Lock()
		persistPath = oldPath
		delete(activeRuns, "test-tracker-unique-42")
		trackerMu.Unlock()
	}()

	trackStart("test-tracker-unique-42", "Test Task", "board", "/tmp/project", "claude", "opus", 12345, "/tmp/test", "/tmp/test.log")

	runs := GetActiveRuns()
	if len(runs) == 0 {
		t.Fatal("should have 1 run")
	}
	if runs[0].Status != "running" {
		t.Errorf("status = %q, want running", runs[0].Status)
	}
	if runs[0].Elapsed == "" {
		t.Error("elapsed should be set for running agent")
	}

	trackEnd("test-tracker-unique-42", 0)
	r := GetRun("test-tracker-unique-42")
	if r.Status != "completed" {
		t.Errorf("status = %q, want completed", r.Status)
	}

	trackEnd("test-tracker-unique-42", 1)
	r = GetRun("test-tracker-unique-42")
	if r.Status != "failed" {
		t.Errorf("status = %q, want failed", r.Status)
	}

	// Cleanup
	trackerMu.Lock()
	delete(activeRuns, "test-tracker-unique-42")
	trackerMu.Unlock()
}
