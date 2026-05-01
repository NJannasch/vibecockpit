package board

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Board struct {
	Name     string       `yaml:"name,omitempty" json:"name"`
	Project  string       `yaml:"project" json:"project"`
	Defaults TaskDefaults `yaml:"defaults,omitempty" json:"defaults"`
	Columns  []string     `yaml:"columns,omitempty" json:"columns"`
	Tasks    []Task       `yaml:"tasks,omitempty" json:"tasks"`

	// Set by discovery, not stored in YAML
	FilePath string `yaml:"-" json:"filePath"`
}

type TaskDefaults struct {
	Tool  string `yaml:"tool,omitempty" json:"tool,omitempty"`
	Model string `yaml:"model,omitempty" json:"model,omitempty"`
}

type Task struct {
	ID          string   `yaml:"id" json:"id"`
	Title       string   `yaml:"title" json:"title"`
	Status      string   `yaml:"status" json:"status"`
	Priority    string   `yaml:"priority,omitempty" json:"priority,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Acceptance  []string `yaml:"acceptance,omitempty" json:"acceptance,omitempty"`
	Tool        string   `yaml:"tool,omitempty" json:"tool,omitempty"`
	Model       string   `yaml:"model,omitempty" json:"model,omitempty"`
	ClaimedBy   string   `yaml:"claimed_by,omitempty" json:"claimedBy,omitempty"`
	Sessions    []string `yaml:"sessions,omitempty" json:"sessions,omitempty"`
	Started     string   `yaml:"started,omitempty" json:"started,omitempty"`
	Completed   string   `yaml:"completed,omitempty" json:"completed,omitempty"`
	Cost        float64  `yaml:"cost,omitempty" json:"cost,omitempty"`
	Summary     string   `yaml:"summary,omitempty" json:"summary,omitempty"`
	CreatedBy   string         `yaml:"created_by,omitempty" json:"createdBy,omitempty"`
	CreatedAt   string         `yaml:"created_at,omitempty" json:"createdAt,omitempty"`
	UpdatedAt   string         `yaml:"updated_at,omitempty" json:"updatedAt,omitempty"`
	History     []HistoryEntry `yaml:"history,omitempty" json:"history,omitempty"`

	// Future fields — parsed but not acted on yet
	MCP          []string `yaml:"mcp,omitempty" json:"mcp,omitempty"`
	Instructions []any    `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	Skills       []string `yaml:"skills,omitempty" json:"skills,omitempty"`
}

type HistoryEntry struct {
	Timestamp string `yaml:"timestamp" json:"timestamp"`
	Action    string `yaml:"action" json:"action"`
	By        string `yaml:"by,omitempty" json:"by,omitempty"`
	Detail    string `yaml:"detail,omitempty" json:"detail,omitempty"`
}

const maxHistory = 10

func (t *Task) RecordHistory(action, by, detail string) {
	t.History = append(t.History, HistoryEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Action:    action,
		By:        by,
		Detail:    detail,
	})
	if len(t.History) > maxHistory {
		t.History = t.History[len(t.History)-maxHistory:]
	}
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

func (t *Task) LinkSession(sessionID, by string) {
	for _, s := range t.Sessions {
		if s == sessionID {
			return
		}
	}
	t.Sessions = append(t.Sessions, sessionID)
	t.RecordHistory("session-linked", by, sessionID)
}

var defaultColumns = []string{"backlog", "claimed", "in-progress", "review", "done"}

func (b *Board) ArchiveTask(taskID, by string) error {
	t, _ := b.FindTask(taskID)
	if t == nil {
		return fmt.Errorf("task %q not found", taskID)
	}
	t.RecordHistory("archived", by, t.Status+" → archived")
	t.Status = "archived"
	return nil
}

func (b *Board) DeleteTask(taskID string) error {
	for i, t := range b.Tasks {
		if t.ID == taskID {
			b.Tasks = append(b.Tasks[:i], b.Tasks[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("task %q not found", taskID)
}

func MoveTaskToBoard(from, to *Board, taskID string) error {
	t, idx := from.FindTask(taskID)
	if t == nil {
		return fmt.Errorf("task %q not found in board %q", taskID, from.Name)
	}
	task := *t
	task.RecordHistory("moved", "", from.Name+" → "+to.Name)
	to.Tasks = append(to.Tasks, task)
	from.Tasks = append(from.Tasks[:idx], from.Tasks[idx+1:]...)
	return nil
}

func (b *Board) ActiveTasks() []Task {
	var out []Task
	for _, t := range b.Tasks {
		if t.Status != "archived" {
			out = append(out, t)
		}
	}
	return out
}

func (b *Board) EffectiveColumns() []string {
	if len(b.Columns) > 0 {
		return b.Columns
	}
	return defaultColumns
}

func (b *Board) TasksByStatus() map[string][]Task {
	m := make(map[string][]Task)
	for _, t := range b.Tasks {
		m[t.Status] = append(m[t.Status], t)
	}
	return m
}

func (b *Board) FindTask(id string) (*Task, int) {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			return &b.Tasks[i], i
		}
	}
	return nil, -1
}

func (b *Board) StatusCounts() map[string]int {
	m := make(map[string]int)
	for _, t := range b.Tasks {
		m[t.Status]++
	}
	return m
}

func Load(path string) (*Board, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b Board
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	b.FilePath = path
	if b.Name == "" {
		base := filepath.Base(path)
		b.Name = strings.TrimSuffix(base, filepath.Ext(base))
		if b.Name == "board" {
			b.Name = filepath.Base(filepath.Dir(filepath.Dir(path)))
		}
	}
	return &b, nil
}

func (b *Board) Save() error {
	if b.FilePath == "" {
		return fmt.Errorf("no file path set")
	}
	if err := os.MkdirAll(filepath.Dir(b.FilePath), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(b)
	if err != nil {
		return err
	}
	return os.WriteFile(b.FilePath, data, 0644)
}

func (b *Board) AddTask(title, priority, description string) Task {
	id := generateID(title)
	for _, t := range b.Tasks {
		if t.ID == id {
			id = id + "-" + fmt.Sprintf("%d", len(b.Tasks)+1)
			break
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	t := Task{
		ID:          id,
		Title:       title,
		Status:      "backlog",
		Priority:    priority,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	b.Tasks = append(b.Tasks, t)
	return t
}

func (b *Board) MoveTask(taskID, newStatus string) error {
	return b.MoveTaskBy(taskID, newStatus, "")
}

func (b *Board) MoveTaskBy(taskID, newStatus, by string) error {
	t, _ := b.FindTask(taskID)
	if t == nil {
		return fmt.Errorf("task %q not found", taskID)
	}
	valid := false
	for _, col := range b.EffectiveColumns() {
		if col == newStatus {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid status %q (valid: %s)", newStatus, strings.Join(b.EffectiveColumns(), ", "))
	}
	oldStatus := t.Status
	t.Status = newStatus
	if newStatus == "done" && t.Completed == "" {
		t.Completed = time.Now().UTC().Format(time.RFC3339)
	}
	if newStatus == "in-progress" && t.Started == "" {
		t.Started = time.Now().UTC().Format(time.RFC3339)
	}
	t.RecordHistory("status", by, oldStatus+" → "+newStatus)
	return nil
}

func Discover(workspaceDir string) ([]*Board, error) {
	var boards []*Board

	// 1. Central boards directory
	centralDir := boardsDir()
	if entries, err := os.ReadDir(centralDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !isYAML(e.Name()) {
				continue
			}
			if b, err := Load(filepath.Join(centralDir, e.Name())); err == nil {
				boards = append(boards, b)
			}
		}
	}

	// 2. Per-project boards
	if workspaceDir != "" {
		expanded := expandHome(workspaceDir)
		if entries, err := os.ReadDir(expanded); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				boardPath := filepath.Join(expanded, e.Name(), ".vibecockpit", "board.yaml")
				if _, err := os.Stat(boardPath); err == nil {
					if b, err := Load(boardPath); err == nil {
						boards = append(boards, b)
					}
				}
			}
		}
	}

	return boards, nil
}

func FindBoard(boards []*Board, name string) *Board {
	for _, b := range boards {
		if b.Name == name {
			return b
		}
	}
	return nil
}

func DeleteBoard(name, workspaceDir string) error {
	boards, err := Discover(workspaceDir)
	if err != nil {
		return err
	}
	b := FindBoard(boards, name)
	if b == nil {
		return fmt.Errorf("board %q not found", name)
	}
	return os.Remove(b.FilePath)
}

func CreateBoard(name, project string) (*Board, error) {
	dir := boardsDir()
	path := filepath.Join(dir, name+".yaml")
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("board %q already exists", name)
	}
	b := &Board{
		Name:     name,
		Project:  project,
		FilePath: path,
	}
	if err := b.Save(); err != nil {
		return nil, err
	}
	return b, nil
}

func boardsDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "vibecockpit", "boards")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vibecockpit", "boards")
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func generateID(title string) string {
	id := strings.ToLower(title)
	id = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, id)
	for strings.Contains(id, "--") {
		id = strings.ReplaceAll(id, "--", "-")
	}
	id = strings.Trim(id, "-")
	if len(id) > 40 {
		id = id[:40]
	}
	if id == "" {
		id = "task"
	}
	return id
}
