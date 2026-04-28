package cursoragent

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"vibecockpit/internal/provider"
)

type CursorAgent struct {
	baseDir string
}

func New() *CursorAgent {
	home, _ := os.UserHomeDir()
	return &CursorAgent{
		baseDir: filepath.Join(home, ".cursor", "chats"),
	}
}

func Available() bool {
	home, _ := os.UserHomeDir()
	_, err := os.Stat(filepath.Join(home, ".cursor", "chats"))
	return err == nil
}

func (c *CursorAgent) Name() string { return "cursor" }
func (c *CursorAgent) Icon() string { return "●" }

func (c *CursorAgent) ResumeCommand(s provider.Session) (string, []string) {
	if strings.HasPrefix(s.Summary, "[ide:") {
		if s.ProjectPath != "" {
			return "cursor", []string{s.ProjectPath}
		}
		return "cursor", nil
	}
	args := []string{"--resume", s.ID}
	if s.ProjectPath != "" {
		args = append(args, "--workspace", s.ProjectPath)
	}
	return "agent", args
}

func (c *CursorAgent) NewCommand(dir string) (string, []string) {
	return "agent", []string{dir}
}

func (c *CursorAgent) DeleteSession(_ string) error {
	return nil
}

func (c *CursorAgent) ScanSessions(_ context.Context) ([]provider.Session, error) {
	workspaceDirs, err := os.ReadDir(c.baseDir)
	if err != nil {
		return nil, err
	}

	var sessions []provider.Session

	for _, wsDir := range workspaceDirs {
		if !wsDir.IsDir() {
			continue
		}
		wsPath := filepath.Join(c.baseDir, wsDir.Name())

		sessDirs, err := os.ReadDir(wsPath)
		if err != nil {
			continue
		}

		for _, sessDir := range sessDirs {
			if !sessDir.IsDir() {
				continue
			}
			dbPath := filepath.Join(wsPath, sessDir.Name(), "store.db")
			if _, err := os.Stat(dbPath); err != nil {
				continue
			}

			s, err := c.scanSession(dbPath, wsDir.Name())
			if err != nil {
				continue
			}
			sessions = append(sessions, *s)
		}
	}

	// Also scan IDE composer sessions from the global state DB
	ideSessions := c.scanIDESessions()
	sessions = append(sessions, ideSessions...)

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})

	return sessions, nil
}

func (c *CursorAgent) scanIDESessions() []provider.Session {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")
	if _, err := os.Stat(dbPath); err != nil {
		// macOS alternative
		dbPath = filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")
		if _, err := os.Stat(dbPath); err != nil {
			return nil
		}
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil
	}
	defer db.Close()

	rows, err := db.Query(`SELECT key, value FROM cursorDiskKV WHERE key LIKE 'composerData:%'`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var sessions []provider.Session
	for rows.Next() {
		var key, value string
		if rows.Scan(&key, &value) != nil {
			continue
		}

		var data struct {
			ComposerID      string `json:"composerId"`
			Name            string `json:"name"`
			Status          string `json:"status"`
			UnifiedMode     string `json:"unifiedMode"`
			CreatedAt       int64  `json:"createdAt"`
			LastUpdatedAt   int64  `json:"lastUpdatedAt"`
			IsAgentic       bool   `json:"isAgentic"`
			ContextUsage    float64 `json:"contextUsagePercent"`
			ModelConfig     struct {
				ModelName string `json:"modelName"`
			} `json:"modelConfig"`
			ConvHeaders     json.RawMessage `json:"fullConversationHeadersOnly"`
			NewFiles        []struct {
				URI struct {
					Path string `json:"path"`
				} `json:"uri"`
			} `json:"newlyCreatedFiles"`
		}
		if json.Unmarshal([]byte(value), &data) != nil {
			continue
		}

		if data.Name == "" || data.CreatedAt == 0 {
			continue
		}

		// Count messages from headers length (each header is ~70 bytes JSON)
		msgCount := len(data.ConvHeaders) / 70

		model := data.ModelConfig.ModelName
		if model == "" || model == "default" {
			model = "cursor-default"
		}

		mode := data.UnifiedMode
		if data.IsAgentic {
			mode = "agent"
		}

		summary := data.Name
		if mode != "" {
			summary = "[ide:" + mode + "] " + data.Name
		}

		// Estimate tokens: ~800 tokens avg per message exchange (industry heuristic)
		estTokens := int64(msgCount) * 800
		ideTokens := provider.TokenUsage{
			InputTokens:  estTokens * 7 / 10,
			OutputTokens: estTokens * 3 / 10,
			TotalTokens:  estTokens,
		}

		// Extract project path from file URIs
		var projectPath string
		for _, f := range data.NewFiles {
			if f.URI.Path != "" {
				projectPath = filepath.Dir(f.URI.Path)
				break
			}
		}

		sessions = append(sessions, provider.Session{
			ID:           data.ComposerID,
			Provider:     "cursor",
			ProjectName:  data.Name,
			ProjectPath:  projectPath,
			Summary:      summary,
			Model:        model,
			MessageCount: msgCount,
			Tokens:       ideTokens,
			Created:      time.UnixMilli(data.CreatedAt),
			Modified:     time.UnixMilli(data.LastUpdatedAt),
		})
	}
	return sessions
}

type sessionMeta struct {
	AgentID       string `json:"agentId"`
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	CreatedAt     int64  `json:"createdAt"`
	LastUsedModel string `json:"lastUsedModel"`
}

func (c *CursorAgent) scanSession(dbPath, workspaceHash string) (*provider.Session, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Read meta
	var meta sessionMeta
	var hexVal string
	err = db.QueryRow("SELECT value FROM meta WHERE key='0'").Scan(&hexVal)
	if err != nil {
		return nil, err
	}

	decoded, err := hex.DecodeString(hexVal)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(decoded, &meta); err != nil {
		return nil, err
	}

	// Count blobs
	var blobCount int
	db.QueryRow("SELECT COUNT(*) FROM blobs").Scan(&blobCount)

	// Extract first user message as summary
	var firstPrompt string
	rows, err := db.Query("SELECT id, data FROM blobs LIMIT 20")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var data []byte
			if rows.Scan(&id, &data) != nil {
				continue
			}
			var msg struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			}
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			if msg.Role == "user" && firstPrompt == "" && !isSystemContent(msg.Content) {
				firstPrompt = cleanPrompt(msg.Content)
				if firstPrompt != "" {
					break
				}
			}
		}
	}

	// Use session name as summary if no user message found
	if firstPrompt == "" {
		firstPrompt = meta.Name
	}
	firstPrompt = "[cli] " + firstPrompt

	// Estimate tokens from user+assistant message sizes only
	// (skip system prompts, tool output, and binary blobs which inflate the count)
	var tokens provider.TokenUsage
	convRows, err := db.Query("SELECT data FROM blobs")
	if err == nil {
		defer convRows.Close()
		for convRows.Next() {
			var data []byte
			if convRows.Scan(&data) != nil {
				continue
			}
			var msg struct{ Role string `json:"role"` }
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			if msg.Role == "user" {
				tokens.InputTokens += int64(len(data)) / 4
			} else if msg.Role == "assistant" {
				tokens.OutputTokens += int64(len(data)) / 4
			}
		}
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens
	}

	created := time.UnixMilli(meta.CreatedAt)

	// Get file modification time for "modified"
	info, _ := os.Stat(dbPath)
	modified := created
	if info != nil {
		modified = info.ModTime()
	}

	projectName := meta.Name
	if projectName == "" {
		projectName = "cursor-session"
	}

	projectPath := extractProjectPath(db)

	return &provider.Session{
		ID:           meta.AgentID,
		Provider:     "cursor",
		ProjectName:  projectName,
		ProjectPath:  projectPath,
		Summary:      firstPrompt,
		FirstPrompt:  firstPrompt,
		Model:        meta.LastUsedModel,
		MessageCount: blobCount,
		Tokens:       tokens,
		Created:      created,
		Modified:     modified,
		DataPath:     dbPath,
	}, nil
}

func extractProjectPath(db *sql.DB) string {
	row := db.QueryRow(`SELECT CAST(data AS TEXT) FROM blobs WHERE CAST(data AS TEXT) LIKE '%Workspace Path:%' LIMIT 1`)
	var text string
	if row.Scan(&text) != nil {
		return ""
	}
	const marker = "Workspace Path: "
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	after := text[idx+len(marker):]
	end := strings.IndexAny(after, "\n\r\"\\")
	if end < 0 {
		return ""
	}
	path := strings.TrimSpace(after[:end])
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return ""
}

func isSystemContent(content any) bool {
	switch v := content.(type) {
	case string:
		return strings.HasPrefix(v, "<user_info>") || strings.HasPrefix(v, "You are")
	}
	return false
}

func cleanPrompt(content any) string {
	var raw string
	switch v := content.(type) {
	case string:
		raw = v
	case []any:
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					raw = t
					break
				}
			}
		}
	}
	// Strip XML-like wrapper tags
	raw = strings.TrimSpace(raw)
	for _, tag := range []string{"<user_query>", "</user_query>", "<user_info>", "</user_info>"} {
		raw = strings.ReplaceAll(raw, tag, "")
	}
	raw = strings.TrimSpace(raw)
	if len(raw) > 150 {
		raw = raw[:150]
	}
	return raw
}
