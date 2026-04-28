package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"vibecockpit/internal/provider"
)

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple header", "# My Project\nsome text", "My Project"},
		{"walkthrough suffix stripped", "# Cool App - Walkthrough", "Cool App"},
		{"walkthrough prefix stripped", "# Walkthrough - Cool App\ntext", "Cool App"},
		{"walkthrough colon prefix stripped", "# Walkthrough: Cool App\ntext", "Cool App"},
		{"task breakdown suffix stripped", "# My App - Task Breakdown", "My App"},
		{"implementation complete suffix stripped", "# App - Implementation Complete", "App"},
		{"no header", "some text\nmore text", ""},
		{"empty input", "", ""},
		{"header after blank lines", "\n\n# Late Header\ntext", "Late Header"},
		{"long title truncated", "# " + string(make([]byte, 120)), string(make([]byte, 100))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitle(tt.input)
			if got != tt.want {
				t.Errorf("extractTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"KlusterKaputt", []string{"klusterkaputt"}},
		{"my-cool-project", []string{"cool", "project"}},
		{"hello_world.app", []string{"hello", "world", "app"}},
		{"a b", nil},
		{"", nil},
		{"vibecockpit.dev (Website)", []string{"vibecockpit", "dev", "website"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("tokenize(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMatchScore(t *testing.T) {
	tests := []struct {
		name     string
		project  []string
		dir      []string
		dirName  string
		wantMin  int
		wantMax  int
	}{
		{
			"exact match",
			[]string{"klusterkaputt"},
			[]string{"klusterkaputt"},
			"klusterkaputt",
			3, 3,
		},
		{
			"partial match substring",
			[]string{"tracker"},
			[]string{"tracker", "frontend"},
			"tracker-frontend",
			1, 4,
		},
		{
			"no match",
			[]string{"something"},
			[]string{"unrelated"},
			"unrelated",
			0, 0,
		},
		{
			"shorter dir preferred via penalty",
			[]string{"klusterkaputt"},
			[]string{"argocd", "klusterkaputt"},
			"argocd-klusterkaputt",
			2, 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchScore(tt.project, tt.dir, tt.dirName)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("matchScore(%v, %v, %q) = %d, want [%d, %d]", tt.project, tt.dir, tt.dirName, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestFuzzyMatchWorkspace(t *testing.T) {
	wsDir := t.TempDir()
	for _, name := range []string{"vibecockpit", "klusterkaputt", "argocd-klusterkaputt", "tracker", "tracker-frontend"} {
		os.MkdirAll(filepath.Join(wsDir, name), 0755)
	}

	a := &Antigravity{workspaceDir: wsDir}

	tests := []struct {
		name     string
		project  string
		wantDir  string
	}{
		{"exact match", "vibecockpit", "vibecockpit"},
		{"shorter dir wins", "KlusterKaputt", "klusterkaputt"},
		{"no match returns empty", "nonexistent-project-xyz", ""},
		{"empty name returns empty", "", ""},
		{"default name returns empty", "antigravity-session", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.fuzzyMatchWorkspace(tt.project)
			if tt.wantDir == "" {
				if got != "" {
					t.Errorf("fuzzyMatchWorkspace(%q) = %q, want empty", tt.project, got)
				}
			} else {
				want := filepath.Join(wsDir, tt.wantDir)
				if got != want {
					t.Errorf("fuzzyMatchWorkspace(%q) = %q, want %q", tt.project, got, want)
				}
			}
		})
	}
}

func TestFuzzyMatchWorkspace_EmptyDir(t *testing.T) {
	a := &Antigravity{workspaceDir: ""}
	got := a.fuzzyMatchWorkspace("some-project")
	if got != "" {
		t.Errorf("expected empty when workspaceDir is empty, got %q", got)
	}
}

func TestBuildSession_WithBrainMetadata(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain", "sess-001")
	os.MkdirAll(brainDir, 0755)

	metaJSON := `{"artifactType":"task","summary":"Build REST API","updatedAt":"2025-06-15T14:30:00Z","version":"1"}`
	os.WriteFile(filepath.Join(brainDir, "task.md.metadata.json"), []byte(metaJSON), 0644)
	os.WriteFile(filepath.Join(brainDir, "walkthrough.md"), []byte("# MyProject\nSome walkthrough"), 0644)

	// Create resolved files to count as message rounds
	os.WriteFile(filepath.Join(brainDir, "task.md.resolved.0"), []byte("resolved"), 0644)
	os.WriteFile(filepath.Join(brainDir, "task.md.resolved.1"), []byte("resolved"), 0644)

	// Create a fake conversation file for ModTime
	convDir := filepath.Join(tmp, "conversations")
	os.MkdirAll(convDir, 0755)
	convPath := filepath.Join(convDir, "sess-001.pb")
	os.WriteFile(convPath, []byte("fake"), 0644)
	convInfo, _ := os.Stat(convPath)

	a := &Antigravity{baseDir: tmp, workspaceDir: ""}

	s := a.buildSession("sess-001", convInfo)

	if s.ID != "sess-001" {
		t.Errorf("ID: got %q, want sess-001", s.ID)
	}
	if s.Provider != "antigravity" {
		t.Errorf("Provider: got %q, want antigravity", s.Provider)
	}
	if s.Summary != "Build REST API" {
		t.Errorf("Summary: got %q, want 'Build REST API'", s.Summary)
	}
	if s.ProjectName != "MyProject" {
		t.Errorf("ProjectName: got %q, want MyProject", s.ProjectName)
	}
	if s.Model != "gemini-2.5-pro" {
		t.Errorf("Model: got %q, want gemini-2.5-pro", s.Model)
	}
	if s.MessageCount != 2 {
		t.Errorf("MessageCount: got %d, want 2", s.MessageCount)
	}
}

func TestBuildSession_FallbackToTaskMd(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain", "sess-002")
	os.MkdirAll(brainDir, 0755)

	os.WriteFile(filepath.Join(brainDir, "task.md"), []byte("# Build Dashboard\nDetails here"), 0644)

	convDir := filepath.Join(tmp, "conversations")
	os.MkdirAll(convDir, 0755)
	convPath := filepath.Join(convDir, "sess-002.pb")
	os.WriteFile(convPath, []byte("fake"), 0644)
	convInfo, _ := os.Stat(convPath)

	a := &Antigravity{baseDir: tmp, workspaceDir: ""}
	s := a.buildSession("sess-002", convInfo)

	if s.ProjectName != "Build Dashboard" {
		t.Errorf("ProjectName: got %q, want 'Build Dashboard'", s.ProjectName)
	}
}

func TestBuildSession_DefaultName(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain", "sess-003")
	os.MkdirAll(brainDir, 0755)

	convDir := filepath.Join(tmp, "conversations")
	os.MkdirAll(convDir, 0755)
	convPath := filepath.Join(convDir, "sess-003.pb")
	os.WriteFile(convPath, []byte("fake"), 0644)
	convInfo, _ := os.Stat(convPath)

	a := &Antigravity{baseDir: tmp, workspaceDir: ""}
	s := a.buildSession("sess-003", convInfo)

	if s.ProjectName != "antigravity-session" {
		t.Errorf("ProjectName: got %q, want 'antigravity-session'", s.ProjectName)
	}
}

func TestScanSessions_Basic(t *testing.T) {
	tmp := t.TempDir()
	convDir := filepath.Join(tmp, "conversations")
	os.MkdirAll(convDir, 0755)

	os.WriteFile(filepath.Join(convDir, "abc.pb"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(convDir, "def.pb"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(convDir, "notapb.txt"), []byte("skip"), 0644)

	a := &Antigravity{baseDir: tmp, workspaceDir: ""}
	sessions, err := a.ScanSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	for _, s := range sessions {
		if s.Provider != "antigravity" {
			t.Errorf("Provider: got %q, want antigravity", s.Provider)
		}
	}
}

func TestScanSessions_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	convDir := filepath.Join(tmp, "conversations")
	os.MkdirAll(convDir, 0755)

	a := &Antigravity{baseDir: tmp, workspaceDir: ""}
	sessions, err := a.ScanSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestScanSessions_MissingDir(t *testing.T) {
	a := &Antigravity{baseDir: "/nonexistent/path", workspaceDir: ""}
	_, err := a.ScanSessions(context.Background())
	if err == nil {
		t.Error("expected error for missing directory")
	}
}

func TestExtractPathFromBrain_WithGitRoot(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	// Create a temporary project directory under $HOME so paths start with /home/ or /Users/
	projectDir := filepath.Join(home, ".cache", "vibecockpit-test-"+t.Name())
	os.MkdirAll(filepath.Join(projectDir, ".git"), 0755)
	os.MkdirAll(filepath.Join(projectDir, "src"), 0755)
	t.Cleanup(func() { os.RemoveAll(projectDir) })

	brainDir := t.TempDir()
	content := "## Implementation\nEdit file at " + filepath.Join(projectDir, "src", "main.go") + " to add handler"
	os.WriteFile(filepath.Join(brainDir, "implementation_plan.md"), []byte(content), 0644)

	got := extractPathFromBrain(brainDir)
	if got != projectDir {
		t.Errorf("extractPathFromBrain() = %q, want %q", got, projectDir)
	}
}

func TestExtractPathFromBrain_NoFiles(t *testing.T) {
	tmp := t.TempDir()
	got := extractPathFromBrain(tmp)
	if got != "" {
		t.Errorf("expected empty for directory with no files, got %q", got)
	}
}

func TestExtractPathFromBrain_SkipsGeminiPaths(t *testing.T) {
	tmp := t.TempDir()
	home, _ := os.UserHomeDir()
	geminiPath := filepath.Join(home, ".gemini", "antigravity", "brain", "something")

	content := "Reference: " + geminiPath + "\n"
	os.WriteFile(filepath.Join(tmp, "walkthrough.md"), []byte(content), 0644)

	got := extractPathFromBrain(tmp)
	if got != "" {
		t.Errorf("expected empty for .gemini paths, got %q", got)
	}
}

func TestResumeCommand(t *testing.T) {
	a := &Antigravity{}

	cmd, args := a.ResumeCommand(provider.Session{ProjectPath: "/home/user/project"})
	if cmd != "antigravity" {
		t.Errorf("cmd: got %q, want antigravity", cmd)
	}
	if len(args) != 1 || args[0] != "/home/user/project" {
		t.Errorf("args: got %v, want [/home/user/project]", args)
	}

	cmd, args = a.ResumeCommand(provider.Session{})
	if cmd != "antigravity" {
		t.Errorf("cmd: got %q, want antigravity", cmd)
	}
	if args != nil {
		t.Errorf("args: got %v, want nil", args)
	}
}

func TestNewCommand(t *testing.T) {
	a := &Antigravity{}
	cmd, args := a.NewCommand("/tmp/new")
	if cmd != "antigravity" {
		t.Errorf("cmd: got %q, want antigravity", cmd)
	}
	if len(args) != 1 || args[0] != "/tmp/new" {
		t.Errorf("args: got %v, want [/tmp/new]", args)
	}
}
