package scheduler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCronValid(t *testing.T) {
	tests := []struct {
		expr string
	}{
		{"* * * * *"},
		{"0 9 * * *"},
		{"*/5 * * * *"},
		{"0 9 * * 1"},
		{"30 6 1 * *"},
		{"0 0 * * 0,6"},
		{"0 9-17 * * 1-5"},
	}
	for _, tt := range tests {
		if _, err := parseCron(tt.expr); err != nil {
			t.Errorf("parseCron(%q) unexpected error: %v", tt.expr, err)
		}
	}
}

func TestParseCronInvalid(t *testing.T) {
	tests := []string{
		"",
		"* * *",
		"* * * * * *",
		"60 * * * *",
		"* 25 * * *",
		"abc * * * *",
	}
	for _, expr := range tests {
		if _, err := parseCron(expr); err == nil {
			t.Errorf("parseCron(%q) should return error", expr)
		}
	}
}

func TestCronMatches(t *testing.T) {
	// Monday 2026-05-04 09:00
	at := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		expr  string
		match bool
	}{
		{"0 9 * * *", true},         // daily at 9am
		{"0 9 * * 1", true},         // Monday 9am
		{"0 10 * * *", false},       // daily at 10am
		{"*/5 * * * *", true},       // every 5 min at :00
		{"0 9 4 * *", true},         // 4th of month at 9am
		{"0 9 5 * *", false},        // 5th of month
		{"* * * * *", true},         // every minute
		{"0 9 * * 0", false},        // Sunday 9am (it's Monday)
	}
	for _, tt := range tests {
		got := cronMatches(tt.expr, at)
		if got != tt.match {
			t.Errorf("cronMatches(%q, %v) = %v, want %v", tt.expr, at, got, tt.match)
		}
	}
}

func TestNextCronTime(t *testing.T) {
	after := time.Date(2026, 5, 4, 8, 55, 0, 0, time.UTC)

	next, err := NextCronTime("0 9 * * *", after)
	if err != nil {
		t.Fatal(err)
	}
	if next.Hour() != 9 || next.Minute() != 0 {
		t.Errorf("expected 09:00, got %s", next.Format("15:04"))
	}
	if !next.After(after) {
		t.Error("next should be after 'after'")
	}
}

func TestCronToHuman(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{"0 9 * * *", "Daily at 9:00"},
		{"*/5 * * * *", "Every 5 minutes"},
		{"0 9 * * 1", "Every Monday at 9:00"},
		{"* * * * *", "Every minute"},
		{"*/2 * * * *", "Every 2 minutes"},
	}
	for _, tt := range tests {
		got := CronToHuman(tt.expr)
		if got != tt.want {
			t.Errorf("CronToHuman(%q) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}

func TestJobCRUD(t *testing.T) {
	s := &Scheduler{
		filePath: filepath.Join(t.TempDir(), "jobs.yaml"),
		running:  make(map[string]bool),
	}

	// Create
	j, err := s.CreateJob(Job{
		Name:    "Test Job",
		Cron:    "0 9 * * *",
		Tool:    "claude",
		Prompt:  "Do something",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if j.ID != "test-job" {
		t.Errorf("expected id 'test-job', got %q", j.ID)
	}

	// List
	jobs := s.GetJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].NextRun == "" {
		t.Error("enabled job should have NextRun")
	}

	// Update
	updated, err := s.UpdateJob("test-job", map[string]any{
		"name":    "Updated Job",
		"enabled": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Updated Job" {
		t.Error("name should be updated")
	}
	if updated.Enabled {
		t.Error("should be disabled")
	}

	// Pause/Resume
	if err := s.ResumeJob("test-job"); err != nil {
		t.Fatal(err)
	}
	j2 := s.GetJob("test-job")
	if !j2.Enabled {
		t.Error("should be enabled after resume")
	}

	if err := s.PauseJob("test-job"); err != nil {
		t.Fatal(err)
	}
	j3 := s.GetJob("test-job")
	if j3.Enabled {
		t.Error("should be disabled after pause")
	}

	// Delete
	if err := s.DeleteJob("test-job"); err != nil {
		t.Fatal(err)
	}
	if len(s.GetJobs()) != 0 {
		t.Error("expected 0 jobs after delete")
	}
}

func TestJobPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.yaml")

	s1 := &Scheduler{filePath: path, running: make(map[string]bool)}
	_, err := s1.CreateJob(Job{
		Name:    "Persist Test",
		Cron:    "0 9 * * *",
		Tool:    "claude",
		Prompt:  "test",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reload from disk
	s2 := &Scheduler{filePath: path, running: make(map[string]bool)}
	s2.load()
	jobs := s2.GetJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job after reload, got %d", len(jobs))
	}
	if jobs[0].Name != "Persist Test" {
		t.Errorf("name = %q, want 'Persist Test'", jobs[0].Name)
	}
}

func TestJobDuplicateID(t *testing.T) {
	s := &Scheduler{
		filePath: filepath.Join(t.TempDir(), "jobs.yaml"),
		running:  make(map[string]bool),
	}
	_, _ = s.CreateJob(Job{Name: "Foo", Cron: "0 9 * * *", Tool: "claude", Prompt: "x"})
	_, err := s.CreateJob(Job{Name: "Foo", Cron: "0 9 * * *", Tool: "claude", Prompt: "y"})
	if err == nil {
		t.Error("duplicate ID should fail")
	}
}

func TestDeleteRunningJob(t *testing.T) {
	s := &Scheduler{
		filePath: filepath.Join(t.TempDir(), "jobs.yaml"),
		running:  map[string]bool{"test": true},
		jobs:     []Job{{ID: "test", Name: "Running"}},
	}
	err := s.DeleteJob("test")
	if err == nil || !strings.Contains(err.Error(), "running") {
		t.Error("should refuse to delete running job")
	}
}

func TestParseFieldStep(t *testing.T) {
	vals, err := parseField("*/15", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	expected := []int{0, 15, 30, 45}
	if len(vals) != len(expected) {
		t.Fatalf("expected %d values, got %d", len(expected), len(vals))
	}
	for i, v := range vals {
		if v != expected[i] {
			t.Errorf("vals[%d] = %d, want %d", i, v, expected[i])
		}
	}
}

func TestParseFieldRange(t *testing.T) {
	vals, err := parseField("9-17", 0, 23)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 9 {
		t.Errorf("expected 9 values (9-17), got %d", len(vals))
	}
}

func TestGenerateID(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Daily Report", "daily-report"},
		{"Board Orchestrator!", "board-orchestrator"},
		{"  spaces  ", "spaces"},
	}
	for _, tt := range tests {
		got := generateID(tt.name)
		if got != tt.want {
			t.Errorf("generateID(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestYAMLPersistFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.yaml")
	s := &Scheduler{filePath: path, running: make(map[string]bool)}
	_, _ = s.CreateJob(Job{
		Name:       "Test",
		Cron:       "0 9 * * *",
		Tool:       "claude",
		Prompt:     "hello",
		Enabled:    true,
		MCPServers: []string{"vibecockpit"},
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "cron:") {
		t.Error("YAML should contain 'cron:' field")
	}
	if !strings.Contains(content, "vibecockpit") {
		t.Error("YAML should contain MCP server name")
	}
}
