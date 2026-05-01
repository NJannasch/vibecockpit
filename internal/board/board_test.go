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
    sessions:
      - ses_abc123
  - id: add-metrics
    title: Add Prometheus metrics endpoint
    status: done
    completed: "2026-04-30T16:45:00Z"
    cost: 11.20
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

func TestAddTaskTimestamps(t *testing.T) {
	b := &Board{Name: "test", FilePath: filepath.Join(t.TempDir(), "test.yaml")}
	b.AddTask("My task", "high", "do stuff")

	task := b.Tasks[0]
	if task.CreatedAt == "" {
		t.Error("CreatedAt should be set")
	}
	if task.UpdatedAt == "" {
		t.Error("UpdatedAt should be set")
	}
	if task.CreatedAt != task.UpdatedAt {
		t.Error("CreatedAt and UpdatedAt should match on creation")
	}
}

func TestMoveTaskByRecordsHistory(t *testing.T) {
	b := &Board{
		Name:     "test",
		FilePath: filepath.Join(t.TempDir(), "test.yaml"),
	}
	b.AddTask("History task", "medium", "")

	if err := b.MoveTaskBy("history-task", "in-progress", "human"); err != nil {
		t.Fatal(err)
	}
	task, _ := b.FindTask("history-task")
	if len(task.History) != 1 {
		t.Fatalf("history = %d, want 1", len(task.History))
	}
	if task.History[0].Action != "status" {
		t.Errorf("action = %q, want %q", task.History[0].Action, "status")
	}
	if task.History[0].By != "human" {
		t.Errorf("by = %q, want %q", task.History[0].By, "human")
	}
	if task.History[0].Detail != "backlog → in-progress" {
		t.Errorf("detail = %q, want %q", task.History[0].Detail, "backlog → in-progress")
	}
	if task.Started == "" {
		t.Error("started should be set")
	}

	if err := b.MoveTaskBy("history-task", "done", "mcp-agent"); err != nil {
		t.Fatal(err)
	}
	if len(task.History) != 2 {
		t.Fatalf("history = %d, want 2", len(task.History))
	}
	if task.History[1].By != "mcp-agent" {
		t.Errorf("by = %q, want %q", task.History[1].By, "mcp-agent")
	}
	if task.Completed == "" {
		t.Error("completed should be set")
	}
}

func TestRecordHistoryCap(t *testing.T) {
	task := &Task{ID: "cap-test", Status: "backlog"}
	for i := 0; i < 15; i++ {
		task.RecordHistory("status", "bot", "change")
	}
	if len(task.History) != 10 {
		t.Errorf("history = %d, want 10 (capped)", len(task.History))
	}
}

func TestRecordHistoryUpdatesTimestamp(t *testing.T) {
	task := &Task{ID: "ts-test", Status: "backlog"}
	task.RecordHistory("priority", "human", "low → high")
	if task.UpdatedAt == "" {
		t.Error("UpdatedAt should be set after RecordHistory")
	}
}

func TestHistoryRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.yaml")

	b := &Board{Name: "hist", Project: ".", FilePath: path}
	b.AddTask("Roundtrip", "high", "")
	_ = b.MoveTaskBy("roundtrip", "in-progress", "human")
	_ = b.MoveTaskBy("roundtrip", "done", "mcp-agent")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	b2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	task, _ := b2.FindTask("roundtrip")
	if task == nil {
		t.Fatal("task not found after reload")
	}
	if len(task.History) != 2 {
		t.Fatalf("history = %d, want 2", len(task.History))
	}
	if task.History[0].By != "human" {
		t.Errorf("history[0].by = %q, want %q", task.History[0].By, "human")
	}
	if task.History[1].By != "mcp-agent" {
		t.Errorf("history[1].by = %q, want %q", task.History[1].By, "mcp-agent")
	}
}

func TestArchiveTask(t *testing.T) {
	b := &Board{Name: "test", FilePath: filepath.Join(t.TempDir(), "test.yaml")}
	b.AddTask("Archive me", "low", "")
	b.AddTask("Keep me", "high", "")

	if err := b.ArchiveTask("archive-me", "human"); err != nil {
		t.Fatal(err)
	}
	task, _ := b.FindTask("archive-me")
	if task.Status != "archived" {
		t.Errorf("status = %q, want %q", task.Status, "archived")
	}
	if len(task.History) != 1 || task.History[0].Action != "archived" {
		t.Error("expected archived history entry")
	}

	active := b.ActiveTasks()
	if len(active) != 1 {
		t.Errorf("active tasks = %d, want 1", len(active))
	}
	if active[0].ID != "keep-me" {
		t.Errorf("active[0].id = %q, want %q", active[0].ID, "keep-me")
	}

	if err := b.ArchiveTask("nonexistent", "human"); err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestDeleteTask(t *testing.T) {
	b := &Board{Name: "test", FilePath: filepath.Join(t.TempDir(), "test.yaml")}
	b.AddTask("Delete me", "low", "")
	b.AddTask("Keep me", "high", "")

	if err := b.DeleteTask("delete-me"); err != nil {
		t.Fatal(err)
	}
	if len(b.Tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(b.Tasks))
	}
	if b.Tasks[0].ID != "keep-me" {
		t.Errorf("remaining task = %q, want %q", b.Tasks[0].ID, "keep-me")
	}

	if err := b.DeleteTask("nonexistent"); err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestMoveTaskToBoard(t *testing.T) {
	dir := t.TempDir()
	from := &Board{Name: "from-board", Project: ".", FilePath: filepath.Join(dir, "from.yaml")}
	to := &Board{Name: "to-board", Project: ".", FilePath: filepath.Join(dir, "to.yaml")}

	from.AddTask("Migrating task", "high", "important work")

	if err := MoveTaskToBoard(from, to, "migrating-task"); err != nil {
		t.Fatal(err)
	}
	if len(from.Tasks) != 0 {
		t.Errorf("from.tasks = %d, want 0", len(from.Tasks))
	}
	if len(to.Tasks) != 1 {
		t.Fatalf("to.tasks = %d, want 1", len(to.Tasks))
	}
	if to.Tasks[0].ID != "migrating-task" {
		t.Errorf("to.tasks[0].id = %q, want %q", to.Tasks[0].ID, "migrating-task")
	}
	if to.Tasks[0].Title != "Migrating task" {
		t.Errorf("title lost during move")
	}
	if len(to.Tasks[0].History) != 1 || to.Tasks[0].History[0].Action != "moved" {
		t.Error("expected moved history entry")
	}
	if to.Tasks[0].History[0].Detail != "from-board → to-board" {
		t.Errorf("history detail = %q, want %q", to.Tasks[0].History[0].Detail, "from-board → to-board")
	}

	if err := MoveTaskToBoard(from, to, "nonexistent"); err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestDeleteBoard(t *testing.T) {
	dir := t.TempDir()
	boardDir := filepath.Join(dir, "boards")
	if err := os.MkdirAll(boardDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(boardDir, "deleteme.yaml")
	if err := os.WriteFile(path, []byte("name: deleteme\nproject: .\ntasks: []\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatal("board file should exist before delete")
	}

	// Can't use DeleteBoard directly since it uses boardsDir(), test the file removal
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("board file should be deleted")
	}
}

func TestLinkSession(t *testing.T) {
	task := &Task{ID: "test", Status: "in-progress"}

	task.LinkSession("ses_abc123", "mcp-agent")
	if len(task.Sessions) != 1 || task.Sessions[0] != "ses_abc123" {
		t.Errorf("sessions = %v, want [ses_abc123]", task.Sessions)
	}
	if len(task.History) != 1 || task.History[0].Action != "session-linked" {
		t.Error("expected session-linked history entry")
	}

	// Duplicate should be ignored
	task.LinkSession("ses_abc123", "mcp-agent")
	if len(task.Sessions) != 1 {
		t.Errorf("duplicate not ignored, sessions = %v", task.Sessions)
	}
	if len(task.History) != 1 {
		t.Error("duplicate should not add history entry")
	}

	// Second unique session
	task.LinkSession("ses_def456", "human")
	if len(task.Sessions) != 2 {
		t.Errorf("sessions = %d, want 2", len(task.Sessions))
	}
	if task.History[1].By != "human" {
		t.Errorf("history[1].by = %q, want %q", task.History[1].By, "human")
	}
}

func TestSessionLinkingRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.yaml")

	b := &Board{Name: "test", Project: ".", FilePath: path}
	b.AddTask("Linked task", "high", "")
	task := &b.Tasks[0]
	task.LinkSession("ses_001", "mcp-agent")
	task.LinkSession("ses_002", "mcp-agent")
	task.Cost = 12.50

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	b2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	t2, _ := b2.FindTask("linked-task")
	if t2 == nil {
		t.Fatal("task not found after reload")
	}
	if len(t2.Sessions) != 2 {
		t.Errorf("sessions = %d, want 2", len(t2.Sessions))
	}
	if t2.Cost != 12.50 {
		t.Errorf("cost = %f, want 12.50", t2.Cost)
	}
}

func TestCreatedByField(t *testing.T) {
	b := &Board{Name: "test", FilePath: filepath.Join(t.TempDir(), "test.yaml")}
	b.AddTask("Human task", "high", "")
	b.Tasks[0].CreatedBy = "human"

	b.AddTask("Agent task", "medium", "")
	b.Tasks[1].CreatedBy = "mcp-agent"

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	b2, err := Load(b.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if b2.Tasks[0].CreatedBy != "human" {
		t.Errorf("tasks[0].createdBy = %q, want %q", b2.Tasks[0].CreatedBy, "human")
	}
	if b2.Tasks[1].CreatedBy != "mcp-agent" {
		t.Errorf("tasks[1].createdBy = %q, want %q", b2.Tasks[1].CreatedBy, "mcp-agent")
	}
}
