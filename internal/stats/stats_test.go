package stats

import (
	"testing"
	"time"

	"vibecockpit/internal/inventory"
	"vibecockpit/internal/provider"
)

func TestCompute(t *testing.T) {
	now := time.Now()
	sessions := []provider.Session{
		{Provider: "claude", ProjectName: "proj-a", Created: now.Add(-30 * 24 * time.Hour), Modified: now.Add(-1 * time.Hour)},
		{Provider: "claude", ProjectName: "proj-a", Created: now.Add(-10 * 24 * time.Hour), Modified: now.Add(-2 * time.Hour)},
		{Provider: "claude", ProjectName: "proj-a", Created: now.Add(-3 * 24 * time.Hour), Modified: now.Add(-30 * time.Minute)},
		{Provider: "claude", ProjectName: "proj-b", Created: now.Add(-5 * 24 * time.Hour), Modified: now},
		{Provider: "codex", ProjectName: "proj-b", Created: now.Add(-20 * 24 * time.Hour), Modified: now.Add(-3 * time.Hour)},
	}

	inv := &inventory.Inventory{
		Tools: []inventory.ToolInfo{
			{ID: "claude", Name: "Claude Code", Installed: true},
			{ID: "codex", Name: "Codex CLI", Installed: true},
			{ID: "aider", Name: "Aider", Installed: false},
		},
		InstructionFiles: []inventory.InstructionFile{
			{Type: "CLAUDE.md", Path: "/proj-a/CLAUDE.md", ProjectName: "proj-a", Modified: now.Add(-25 * 24 * time.Hour).Format(time.RFC3339)},
		},
		IDEExtensions: []inventory.IDEExtension{
			{ID: "github.copilot", Name: "GitHub Copilot", IDE: "VS Code", Installed: now.Add(-60 * 24 * time.Hour).Format(time.RFC3339)},
		},
		Memories: []inventory.MemoryFile{
			{Name: "user-profile", MemoryType: "user", ProjectName: "proj-a", Modified: now.Add(-15 * 24 * time.Hour).Format(time.RFC3339)},
		},
		ProjectPaths: []string{"/proj-a", "/proj-b"},
	}

	result := Compute(sessions, inv)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	t.Run("summary", func(t *testing.T) {
		s := result.Summary
		if s.TotalSessions != 5 {
			t.Errorf("totalSessions = %d, want 5", s.TotalSessions)
		}
		if s.MostActiveTool != "claude" {
			t.Errorf("mostActiveTool = %q, want claude", s.MostActiveTool)
		}
		if s.MostActiveProject != "proj-a" {
			t.Errorf("mostActiveProject = %q, want proj-a (3 sessions)", s.MostActiveProject)
		}
		if s.DaysSinceFirstUse < 29 {
			t.Errorf("daysSinceFirstUse = %d, want >= 29", s.DaysSinceFirstUse)
		}
		if s.TotalTools != 3 {
			t.Errorf("totalTools = %d, want 3", s.TotalTools)
		}
		if s.TotalExtensions != 1 {
			t.Errorf("totalExtensions = %d", s.TotalExtensions)
		}
		if s.ProjectsWithConfigs != 1 {
			t.Errorf("projectsWithConfigs = %d, want 1", s.ProjectsWithConfigs)
		}
	})

	t.Run("tools", func(t *testing.T) {
		if len(result.Tools) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(result.Tools))
		}
		if result.Tools[0].Name != "claude" {
			t.Errorf("first tool = %q, want claude (most sessions)", result.Tools[0].Name)
		}
		if result.Tools[0].TotalSessions != 4 {
			t.Errorf("claude sessions = %d, want 4", result.Tools[0].TotalSessions)
		}
	})

	t.Run("timeline sorted ascending", func(t *testing.T) {
		if len(result.Timeline) == 0 {
			t.Fatal("expected timeline events")
		}
		for i := 1; i < len(result.Timeline); i++ {
			if result.Timeline[i].Date < result.Timeline[i-1].Date {
				t.Errorf("timeline not sorted: [%d]=%s > [%d]=%s", i-1, result.Timeline[i-1].Date, i, result.Timeline[i].Date)
			}
		}
	})

	t.Run("timeline event types", func(t *testing.T) {
		types := map[string]int{}
		for _, ev := range result.Timeline {
			types[ev.Type]++
		}
		if types["first-session"] != 2 {
			t.Errorf("expected 2 first-session events, got %d", types["first-session"])
		}
		if types["instruction-file"] != 1 {
			t.Errorf("expected 1 instruction-file event, got %d", types["instruction-file"])
		}
		if types["extension-install"] != 1 {
			t.Errorf("expected 1 extension-install event, got %d", types["extension-install"])
		}
		if types["memory-created"] != 1 {
			t.Errorf("expected 1 memory-created event, got %d", types["memory-created"])
		}
	})

	t.Run("artifacts", func(t *testing.T) {
		if len(result.Artifacts) != 3 {
			t.Errorf("expected 3 artifacts, got %d", len(result.Artifacts))
		}
	})
}

func TestComputeNilInventory(t *testing.T) {
	result := Compute(nil, nil)
	if result == nil {
		t.Fatal("expected non-nil result even with nil inputs")
	}
	if len(result.Timeline) != 0 {
		t.Errorf("expected empty timeline, got %d", len(result.Timeline))
	}
}

func TestComputeEmptySessions(t *testing.T) {
	inv := &inventory.Inventory{
		InstructionFiles: []inventory.InstructionFile{
			{Type: "CLAUDE.md", Modified: time.Now().Format(time.RFC3339), ProjectName: "p"},
		},
	}
	result := Compute(nil, inv)
	if len(result.Timeline) != 1 {
		t.Errorf("expected 1 timeline event from instruction file, got %d", len(result.Timeline))
	}
	if result.Summary.TotalSessions != 0 {
		t.Errorf("expected 0 sessions")
	}
}

func TestComputeSessionsWithoutCreated(t *testing.T) {
	now := time.Now()
	sessions := []provider.Session{
		{Provider: "claude", Modified: now.Add(-5 * 24 * time.Hour)},
	}
	result := Compute(sessions, nil)
	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].FirstSession == "" {
		t.Error("should fall back to Modified when Created is zero")
	}
}
