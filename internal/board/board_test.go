package board

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleBoard = `name: test-board
project: ~/Projects/test
defaults:
  tool: claude-code
  model: opus-4.6

tasks:
  - id: auth-flow
    title: Add OAuth2 PKCE flow
    status: backlog
    priority: high
    description: |
      Implement OAuth2 PKCE flow using existing auth middleware.
    acceptance:
      - PKCE challenge/verifier generated per request
      - Token refresh works without re-auth
  - id: fix-webhook
    title: Fix Stripe webhook retry logic
    status: in-progress
    claimed_by: claude-code
    session: ses_abc123
  - id: add-metrics
    title: Add Prometheus metrics endpoint
    status: done
    completed: "2026-04-30T16:45:00Z"
    cost: "$11.20"
`

func TestLoadBoard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-board.yaml")
	if err := os.WriteFile(path, []byte(sampleBoard), 0644); err != nil {
		t.Fatal(err)
	}

	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "test-board" {
		t.Errorf("name = %q, want %q", b.Name, "test-board")
	}
	if b.Project != "~/Projects/test" {
		t.Errorf("project = %q, want %q", b.Project, "~/Projects/test")
	}
	if b.Defaults.Tool != "claude-code" {
		t.Errorf("defaults.tool = %q, want %q", b.Defaults.Tool, "claude-code")
	}
	if len(b.Tasks) != 3 {
		t.Fatalf("tasks = %d, want 3", len(b.Tasks))
	}
	if b.Tasks[0].ID != "auth-flow" {
		t.Errorf("tasks[0].id = %q, want %q", b.Tasks[0].ID, "auth-flow")
	}
	if b.Tasks[0].Priority != "high" {
		t.Errorf("tasks[0].priority = %q, want %q", b.Tasks[0].Priority, "high")
	}
	if len(b.Tasks[0].Acceptance) != 2 {
		t.Errorf("tasks[0].acceptance = %d, want 2", len(b.Tasks[0].Acceptance))
	}
}

func TestBoardNameFromFilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-project.yaml")
	if err := os.WriteFile(path, []byte("project: ~/test\ntasks: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "my-project" {
		t.Errorf("name = %q, want %q", b.Name, "my-project")
	}
}

func TestBoardNameFromProjectDir(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "cool-project", ".vibecockpit")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projDir, "board.yaml")
	if err := os.WriteFile(path, []byte("project: ~/test\ntasks: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "cool-project" {
		t.Errorf("name = %q, want %q", b.Name, "cool-project")
	}
}

func TestFindTask(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(sampleBoard), 0644); err != nil {
		t.Fatal(err)
	}
	b, _ := Load(path)

	task, idx := b.FindTask("fix-webhook")
	if task == nil || idx != 1 {
		t.Fatalf("FindTask(fix-webhook) = %v, %d", task, idx)
	}
	if task.ClaimedBy != "claude-code" {
		t.Errorf("claimed_by = %q, want %q", task.ClaimedBy, "claude-code")
	}

	task, idx = b.FindTask("nonexistent")
	if task != nil || idx != -1 {
		t.Errorf("FindTask(nonexistent) = %v, %d, want nil, -1", task, idx)
	}
}

func TestAddTask(t *testing.T) {
	b := &Board{Name: "test", FilePath: filepath.Join(t.TempDir(), "test.yaml")}
	task := b.AddTask("Fix login bug", "high", "Users can't log in with SSO")

	if task.ID != "fix-login-bug" {
		t.Errorf("id = %q, want %q", task.ID, "fix-login-bug")
	}
	if task.Status != "backlog" {
		t.Errorf("status = %q, want %q", task.Status, "backlog")
	}
	if len(b.Tasks) != 1 {
		t.Errorf("tasks count = %d, want 1", len(b.Tasks))
	}
}

func TestMoveTask(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(sampleBoard), 0644); err != nil {
		t.Fatal(err)
	}
	b, _ := Load(path)

	if err := b.MoveTask("auth-flow", "in-progress"); err != nil {
		t.Fatal(err)
	}
	task, _ := b.FindTask("auth-flow")
	if task.Status != "in-progress" {
		t.Errorf("status = %q, want %q", task.Status, "in-progress")
	}
	if task.Started == "" {
		t.Error("started should be set")
	}

	if err := b.MoveTask("auth-flow", "done"); err != nil {
		t.Fatal(err)
	}
	if task.Completed == "" {
		t.Error("completed should be set")
	}

	if err := b.MoveTask("auth-flow", "invalid"); err == nil {
		t.Error("expected error for invalid status")
	}

	if err := b.MoveTask("nonexistent", "done"); err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	b := &Board{
		Name:     "roundtrip",
		Project:  "~/Projects/test",
		Defaults: TaskDefaults{Tool: "claude-code"},
		FilePath: path,
	}
	b.AddTask("Task one", "high", "Do the thing")
	b.AddTask("Task two", "", "")

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	b2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if b2.Name != "roundtrip" {
		t.Errorf("name = %q, want %q", b2.Name, "roundtrip")
	}
	if len(b2.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(b2.Tasks))
	}
	if b2.Tasks[0].Title != "Task one" {
		t.Errorf("tasks[0].title = %q, want %q", b2.Tasks[0].Title, "Task one")
	}
}

func TestStatusCounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(sampleBoard), 0644); err != nil {
		t.Fatal(err)
	}
	b, _ := Load(path)
	counts := b.StatusCounts()
	if counts["backlog"] != 1 {
		t.Errorf("backlog = %d, want 1", counts["backlog"])
	}
	if counts["in-progress"] != 1 {
		t.Errorf("in-progress = %d, want 1", counts["in-progress"])
	}
	if counts["done"] != 1 {
		t.Errorf("done = %d, want 1", counts["done"])
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()

	// Create a per-project board
	projBoard := filepath.Join(dir, "my-project", ".vibecockpit")
	if err := os.MkdirAll(projBoard, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projBoard, "board.yaml"), []byte("project: ./\ntasks:\n  - id: t1\n    title: Test\n    status: backlog\n"), 0644); err != nil {
		t.Fatal(err)
	}

	boards, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) < 1 {
		t.Fatal("expected at least 1 board from project discovery")
	}

	found := false
	for _, b := range boards {
		if b.Name == "my-project" {
			found = true
			if len(b.Tasks) != 1 {
				t.Errorf("tasks = %d, want 1", len(b.Tasks))
			}
		}
	}
	if !found {
		t.Error("did not discover my-project board")
	}
}

func TestGenerateID(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Fix login bug", "fix-login-bug"},
		{"Add OAuth2 PKCE flow", "add-oauth2-pkce-flow"},
		{"  spaces  and -- dashes  ", "spaces-and-dashes"},
		{"UPPERCASE", "uppercase"},
		{"", "task"},
	}
	for _, tt := range tests {
		got := generateID(tt.title)
		if got != tt.want {
			t.Errorf("generateID(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}
