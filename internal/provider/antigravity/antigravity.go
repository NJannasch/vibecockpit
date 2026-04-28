package antigravity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vibecockpit/internal/provider"
)

type Antigravity struct {
	baseDir      string
	workspaceDir string
}

func New(workspaceDir string) *Antigravity {
	home, _ := os.UserHomeDir()
	return &Antigravity{
		baseDir:      filepath.Join(home, ".gemini", "antigravity"),
		workspaceDir: workspaceDir,
	}
}

func Available() bool {
	home, _ := os.UserHomeDir()
	_, err := os.Stat(filepath.Join(home, ".gemini", "antigravity", "conversations"))
	return err == nil
}

func (a *Antigravity) Name() string { return "antigravity" }
func (a *Antigravity) Icon() string { return "▲" }

func (a *Antigravity) ResumeCommand(s provider.Session) (string, []string) {
	if s.ProjectPath != "" {
		return "antigravity", []string{s.ProjectPath}
	}
	return "antigravity", nil
}

func (a *Antigravity) NewCommand(dir string) (string, []string) {
	return "antigravity", []string{dir}
}

func (a *Antigravity) DeleteSession(_ string) error {
	return nil
}

type brainMeta struct {
	ArtifactType string `json:"artifactType"`
	Summary      string `json:"summary"`
	UpdatedAt    string `json:"updatedAt"`
	Version      string `json:"version"`
}

func (a *Antigravity) ScanSessions(_ context.Context) ([]provider.Session, error) {
	convDir := filepath.Join(a.baseDir, "conversations")
	entries, err := os.ReadDir(convDir)
	if err != nil {
		return nil, err
	}

	var sessions []provider.Session

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pb") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".pb")
		convPath := filepath.Join(convDir, entry.Name())

		info, err := os.Stat(convPath)
		if err != nil {
			continue
		}

		s := a.buildSession(id, info)
		sessions = append(sessions, s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})

	return sessions, nil
}

func (a *Antigravity) buildSession(id string, convInfo os.FileInfo) provider.Session {
	modified := convInfo.ModTime()
	created := modified

	projectName := "antigravity-session"
	summary := ""
	var messageCount int

	brainDir := filepath.Join(a.baseDir, "brain", id)

	// Try task metadata first
	metaPath := filepath.Join(brainDir, "task.md.metadata.json")
	if data, err := os.ReadFile(metaPath); err == nil {
		var meta brainMeta
		if json.Unmarshal(data, &meta) == nil {
			if meta.Summary != "" {
				summary = meta.Summary
			}
			if meta.UpdatedAt != "" {
				if t, err := time.Parse(time.RFC3339Nano, meta.UpdatedAt); err == nil {
					modified = t
				}
			}
		}
	}

	// Prefer walkthrough.md for project name (has actual project names)
	walkPath := filepath.Join(brainDir, "walkthrough.md")
	if data, err := os.ReadFile(walkPath); err == nil {
		if name := extractTitle(string(data)); name != "" {
			projectName = name
		}
	}

	// Fallback: try task.md header (often generic like "Tasks")
	if projectName == "antigravity-session" {
		taskPath := filepath.Join(brainDir, "task.md")
		if data, err := os.ReadFile(taskPath); err == nil {
			if name := extractTitle(string(data)); name != "" {
				projectName = name
			}
		}
	}

	// Count task.md.resolved.* files as a proxy for interaction rounds
	if entries, err := os.ReadDir(brainDir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "task.md.resolved.") {
				messageCount++
			}
		}
	}

	// Use earliest resolved file as created time
	resolvedPath := filepath.Join(brainDir, "task.md.resolved.0")
	if info, err := os.Stat(resolvedPath); err == nil {
		created = info.ModTime()
	} else if info, err := os.Stat(filepath.Join(brainDir, "task.md")); err == nil {
		created = info.ModTime()
	}

	// Try to extract project path from brain file contents (file paths in plans/walkthroughs)
	projectPath := extractPathFromBrain(brainDir)
	if projectPath == "" {
		projectPath = a.fuzzyMatchWorkspace(projectName)
	}

	return provider.Session{
		ID:           id,
		Provider:     "antigravity",
		ProjectName:  projectName,
		ProjectPath:  projectPath,
		Summary:      summary,
		Model:        "gemini-2.5-pro",
		MessageCount: messageCount,
		Created:      created,
		Modified:     modified,
		DataPath:     filepath.Join(a.baseDir, "conversations", id+".pb"),
	}
}

func extractPathFromBrain(brainDir string) string {
	// Scan main files first, then resolved versions
	var files []string
	for _, name := range []string{"walkthrough.md", "implementation_plan.md", "task.md"} {
		files = append(files, name)
	}
	if entries, err := os.ReadDir(brainDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".resolved") || strings.Contains(e.Name(), ".resolved.") {
				files = append(files, e.Name())
			}
		}
	}

	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(brainDir, name))
		if err != nil {
			continue
		}
		content := string(data)
		// Look for absolute paths like /home/user/... or /Users/user/...
		for _, line := range strings.Split(content, "\n") {
			idx := strings.Index(line, "/home/")
			if idx < 0 {
				idx = strings.Index(line, "/Users/")
			}
			if idx < 0 {
				continue
			}
			// Extract the path, stopping at common delimiters
			path := line[idx:]
			// Trim trailing markdown/parens
			for _, suffix := range []string{")", "]", "`", "\"", "'"} {
				if i := strings.Index(path, suffix); i > 0 {
					path = path[:i]
				}
			}
			// Skip paths inside ~/.gemini (brain artifacts, not project files)
			if strings.Contains(path, "/.gemini/") {
				continue
			}
			// Walk up to find the project root — prefer .git, then other markers
			dir := filepath.Dir(path)
			// First pass: look for .git (definitive project root)
			candidate := dir
			for candidate != "/" && candidate != "." {
				if _, err := os.Stat(filepath.Join(candidate, ".git")); err == nil {
					return candidate
				}
				candidate = filepath.Dir(candidate)
			}
			// Second pass: find shallowest directory with a project marker
			candidate = dir
			var found string
			for candidate != "/" && candidate != "." {
				if info, err := os.Stat(candidate); err == nil && info.IsDir() {
					for _, marker := range []string{"package.json", "go.mod", "Cargo.toml", "pyproject.toml", "Makefile", "docker-compose.yml", "Dockerfile", "README.md"} {
						if _, err := os.Stat(filepath.Join(candidate, marker)); err == nil {
							found = candidate
						}
					}
				}
				candidate = filepath.Dir(candidate)
			}
			if found != "" {
				return found
			}
			// Fallback: just use the parent of the referenced file if it exists
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	return ""
}

func (a *Antigravity) fuzzyMatchWorkspace(projectName string) string {
	if projectName == "" || projectName == "antigravity-session" || a.workspaceDir == "" {
		return ""
	}

	searchDirs := []string{a.workspaceDir}

	// Build keywords from project name: split on spaces, parens, dashes
	keywords := tokenize(projectName)
	if len(keywords) == 0 {
		return ""
	}

	var bestMatch string
	bestScore := 0

	for _, searchDir := range searchDirs {
		entries, err := os.ReadDir(searchDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dirTokens := tokenize(e.Name())
			score := matchScore(keywords, dirTokens, e.Name())
			if score > bestScore {
				bestScore = score
				bestMatch = filepath.Join(searchDir, e.Name())
			}
		}
	}

	if bestScore >= 2 || (bestScore == 1 && len(keywords) == 1) {
		return bestMatch
	}
	return ""
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	// Replace common separators with spaces
	for _, c := range []string{"-", "_", ".", "(", ")", ",", ":", "!", "/"} {
		s = strings.ReplaceAll(s, c, " ")
	}
	var tokens []string
	for _, w := range strings.Fields(s) {
		if len(w) >= 3 { // skip very short words
			tokens = append(tokens, w)
		}
	}
	return tokens
}

func matchScore(projectKeywords, dirTokens []string, dirName string) int {
	score := 0
	for _, kw := range projectKeywords {
		for _, dt := range dirTokens {
			if kw == dt {
				score += 3
			} else if strings.Contains(dt, kw) || strings.Contains(kw, dt) {
				score += 2
			}
		}
	}
	// Prefer shorter (more specific) directory names on equal match score
	// by penalizing extra unmatched tokens
	unmatched := len(dirTokens) - len(projectKeywords)
	if unmatched > 0 && score > 0 {
		score -= unmatched
	}
	return score
}

func extractTitle(md string) string {
	for _, line := range strings.SplitN(md, "\n", 5) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimPrefix(line, "# ")
			for _, suffix := range []string{" - Task Breakdown", " - Task List", " - Implementation Complete", " - Walkthrough", " Walkthrough"} {
				title = strings.TrimSuffix(title, suffix)
			}
			for _, prefix := range []string{"Walkthrough - ", "Walkthrough: "} {
				title = strings.TrimPrefix(title, prefix)
			}
			if len(title) > 100 {
				title = title[:100]
			}
			return title
		}
	}
	return ""
}
