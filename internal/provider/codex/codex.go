package codex

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"vibecockpit/internal/provider"
)

type Codex struct {
	dbPath string
}

func New() *Codex {
	home, _ := os.UserHomeDir()
	return &Codex{
		dbPath: filepath.Join(home, ".codex", "state_5.sqlite"),
	}
}

func (c *Codex) Available() bool {
	_, err := os.Stat(c.dbPath)
	return err == nil
}

func (c *Codex) Name() string { return "codex" }
func (c *Codex) Icon() string { return "●" }

func (c *Codex) ResumeCommand(s provider.Session) (string, []string) {
	return "codex", []string{"resume", s.ID}
}

func (c *Codex) NewCommand(dir string) (string, []string) {
	return "codex", nil
}

func (c *Codex) ScanSessions(ctx context.Context) ([]provider.Session, error) {
	db, err := c.open(true)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT
			id, title, cwd, model_provider, model,
			created_at, updated_at, tokens_used,
			first_user_message, git_branch, cli_version, archived
		FROM threads
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []provider.Session
	for rows.Next() {
		var (
			id, title, cwd                         sql.NullString
			modelProvider, model                    sql.NullString
			createdAt, updatedAt                    sql.NullInt64
			tokensUsed                              sql.NullInt64
			firstUserMessage, gitBranch, cliVersion sql.NullString
			archived                                sql.NullInt64
		)
		if err := rows.Scan(&id, &title, &cwd, &modelProvider, &model,
			&createdAt, &updatedAt, &tokensUsed,
			&firstUserMessage, &gitBranch, &cliVersion, &archived); err != nil {
			continue
		}

		if archived.Valid && archived.Int64 != 0 {
			continue
		}

		modelStr := ""
		if model.Valid && model.String != "" {
			modelStr = model.String
		}
		if modelStr == "" && modelProvider.Valid {
			modelStr = modelProvider.String
		}
		if modelProvider.Valid && model.Valid && model.String != "" {
			modelStr = modelProvider.String + "/" + model.String
		}

		projectPath := cwd.String
		var tokens provider.TokenUsage
		if tokensUsed.Valid {
			tokens.TotalTokens = tokensUsed.Int64
		}
		s := provider.Session{
			ID:          id.String,
			Provider:    "codex",
			ProjectName: filepath.Base(projectPath),
			ProjectPath: projectPath,
			Summary:     title.String,
			FirstPrompt: firstUserMessage.String,
			Model:       modelStr,
			GitBranch:   gitBranch.String,
			Tokens:      tokens,
		}

		if createdAt.Valid {
			s.Created = time.Unix(createdAt.Int64, 0)
		}
		if updatedAt.Valid {
			s.Modified = time.Unix(updatedAt.Int64, 0)
		}

		sessions = append(sessions, s)
	}

	// Batch-count messages per session from history.jsonl
	counts := countMessagesFromHistory()
	for i := range sessions {
		if n, ok := counts[sessions[i].ID]; ok {
			sessions[i].MessageCount = n
		}
	}

	return sessions, nil
}

func (c *Codex) DeleteSession(sessionID string) error {
	db, err := c.open(false)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"thread_dynamic_tools", "stage1_outputs", "thread_spawn_edges"} {
		tx.Exec("DELETE FROM "+table+" WHERE thread_id = ?", sessionID)
	}
	if _, err := tx.Exec("DELETE FROM threads WHERE id = ?", sessionID); err != nil {
		return fmt.Errorf("deleting thread: %w", err)
	}

	return tx.Commit()
}

func (c *Codex) open(readOnly bool) (*sql.DB, error) {
	dsn := c.dbPath
	if readOnly {
		dsn += "?mode=ro"
	}
	return sql.Open("sqlite", dsn)
}

func countMessagesFromHistory() map[string]int {
	home, _ := os.UserHomeDir()
	f, err := os.Open(filepath.Join(home, ".codex", "history.jsonl"))
	if err != nil {
		return nil
	}
	defer f.Close()

	counts := make(map[string]int)
	buf := make([]byte, 0, 4096)
	readBuf := make([]byte, 32768)

	for {
		n, err := f.Read(readBuf)
		if n > 0 {
			buf = append(buf, readBuf[:n]...)
		}
		if err != nil {
			break
		}
	}

	// Fast scan: find "session_id":"<uuid>" patterns without full JSON parsing
	key := []byte(`"session_id":"`)
	for i := 0; i < len(buf); i++ {
		if buf[i] == '\n' {
			// Count one message per line, extract session_id
			lineStart := 0
			if i > 0 {
				for j := i - 1; j >= 0 && buf[j] != '\n'; j-- {
					lineStart = j
				}
			}
			line := buf[lineStart:i]
			if idx := indexOf(line, key); idx >= 0 {
				start := idx + len(key)
				end := start
				for end < len(line) && line[end] != '"' {
					end++
				}
				if end > start {
					sid := string(line[start:end])
					counts[sid]++
				}
			}
		}
	}

	return counts
}

func indexOf(haystack, needle []byte) int {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
