package gemini

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

type Gemini struct {
	baseDir string
}

func New() *Gemini {
	home, _ := os.UserHomeDir()
	return &Gemini{
		baseDir: filepath.Join(home, ".gemini"),
	}
}

func (g *Gemini) Available() bool {
	_, err := os.Stat(filepath.Join(g.baseDir, "tmp"))
	return err == nil
}

func (g *Gemini) Name() string { return "gemini" }
func (g *Gemini) Icon() string { return "●" }

func (g *Gemini) ResumeCommand(s provider.Session) (string, []string) {
	return "gemini", nil
}

func (g *Gemini) NewCommand(dir string) (string, []string) {
	return "gemini", nil
}

func (g *Gemini) ScanSessions(_ context.Context) ([]provider.Session, error) {
	projects := g.loadProjects()

	var sessions []provider.Session
	tmpDir := filepath.Join(g.baseDir, "tmp")

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		chatsDir := filepath.Join(tmpDir, e.Name(), "chats")
		chatFiles, err := os.ReadDir(chatsDir)
		if err != nil {
			continue
		}

		projectPath := projects[e.Name()]
		if projectPath == "" {
			projectPath = resolveProjectRoot(filepath.Join(g.baseDir, "history", e.Name()))
		}

		for _, cf := range chatFiles {
			if !strings.HasSuffix(cf.Name(), ".json") {
				continue
			}
			s, err := g.parseSession(filepath.Join(chatsDir, cf.Name()), e.Name(), projectPath)
			if err != nil {
				continue
			}
			sessions = append(sessions, *s)
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})

	return sessions, nil
}

func (g *Gemini) DeleteSession(sessionID string) error {
	tmpDir := filepath.Join(g.baseDir, "tmp")
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		chatsDir := filepath.Join(tmpDir, e.Name(), "chats")
		chatFiles, _ := os.ReadDir(chatsDir)
		for _, cf := range chatFiles {
			path := filepath.Join(chatsDir, cf.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var sess sessionFile
			if json.Unmarshal(data, &sess) == nil && sess.SessionID == sessionID {
				return os.Remove(path)
			}
		}
	}
	return os.ErrNotExist
}

type sessionFile struct {
	SessionID   string    `json:"sessionId"`
	ProjectHash string    `json:"projectHash"`
	StartTime   string    `json:"startTime"`
	LastUpdated string    `json:"lastUpdated"`
	Messages    []message `json:"messages"`
}

type message struct {
	Type      string     `json:"type"`
	Content   string     `json:"content"`
	Model     string     `json:"model,omitempty"`
	ToolCalls []toolCall `json:"toolCalls,omitempty"`
	Tokens    *msgTokens `json:"tokens,omitempty"`
}

type msgTokens struct {
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Cached   int64 `json:"cached"`
	Thoughts int64 `json:"thoughts"`
	Total    int64 `json:"total"`
}

type toolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

func (g *Gemini) parseSession(path, _, projectPath string) (*provider.Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var sf sessionFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, err
	}

	created, _ := time.Parse(time.RFC3339Nano, sf.StartTime)
	modified, _ := time.Parse(time.RFC3339Nano, sf.LastUpdated)

	var firstPrompt, model string
	var tokens provider.TokenUsage
	msgCount := 0

	for _, m := range sf.Messages {
		if m.Type == "user" || m.Type == "gemini" {
			msgCount++
		}
		if m.Type == "user" && firstPrompt == "" {
			firstPrompt = m.Content
			if len(firstPrompt) > 200 {
				firstPrompt = firstPrompt[:200]
			}
		}
		if m.Model != "" {
			model = m.Model
		}
		if m.Tokens != nil {
			tokens.InputTokens += m.Tokens.Input
			tokens.OutputTokens += m.Tokens.Output
			tokens.CacheReadTokens += m.Tokens.Cached
			tokens.ReasoningTokens += m.Tokens.Thoughts
			tokens.TotalTokens += m.Tokens.Total
		}
		if projectPath == "" {
			projectPath = extractPathFromMessage(m)
		}
	}

	projName := filepath.Base(projectPath)
	if projectPath == "" {
		projName = "gemini-session"
	}
	if model == "" {
		model = "gemini"
	}

	return &provider.Session{
		ID:           sf.SessionID,
		Provider:     "gemini",
		ProjectName:  projName,
		ProjectPath:  projectPath,
		Summary:      truncateFirstLine(firstPrompt),
		FirstPrompt:  firstPrompt,
		Model:        model,
		MessageCount: msgCount,
		Tokens:       tokens,
		Created:      created,
		Modified:     modified,
	}, nil
}

func extractPathFromMessage(m message) string {
	for _, tc := range m.ToolCalls {
		if fp, ok := tc.Args["file_path"].(string); ok && strings.HasPrefix(fp, "/") {
			return guessProjectRoot(fp)
		}
		if fp, ok := tc.Args["path"].(string); ok && strings.HasPrefix(fp, "/") {
			return guessProjectRoot(fp)
		}
	}
	return ""
}

func guessProjectRoot(filePath string) string {
	parts := strings.Split(filePath, "/")
	for i := len(parts) - 1; i >= 3; i-- {
		candidate := strings.Join(parts[:i], "/")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			if _, err := os.Stat(candidate + "/.git"); err == nil {
				return candidate
			}
		}
	}
	if len(parts) >= 5 {
		return strings.Join(parts[:5], "/")
	}
	return ""
}

func (g *Gemini) loadProjects() map[string]string {
	data, err := os.ReadFile(filepath.Join(g.baseDir, "projects.json"))
	if err != nil {
		return nil
	}
	var pf struct {
		Projects map[string]string `json:"projects"`
	}
	if json.Unmarshal(data, &pf) != nil {
		return nil
	}
	// Reverse: path→name to name→path
	result := make(map[string]string)
	for path, name := range pf.Projects {
		result[name] = path
	}
	return result
}

func resolveProjectRoot(historyDir string) string {
	data, err := os.ReadFile(filepath.Join(historyDir, ".project_root"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func truncateFirstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}
