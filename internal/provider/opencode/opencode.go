package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"vibecockpit/internal/provider"
)

type OpenCode struct {
	dbPath string
}

func New() *OpenCode {
	home, _ := os.UserHomeDir()
	return &OpenCode{
		dbPath: filepath.Join(home, ".local", "share", "opencode", "opencode.db"),
	}
}

func (o *OpenCode) Available() bool {
	_, err := os.Stat(o.dbPath)
	return err == nil
}

func (o *OpenCode) Name() string { return "opencode" }
func (o *OpenCode) Icon() string { return "●" }

func (o *OpenCode) ResumeCommand(s provider.Session) (string, []string) {
	return "opencode", nil
}

func (o *OpenCode) NewCommand(dir string) (string, []string) {
	return "opencode", nil
}

func (o *OpenCode) ScanSessions(ctx context.Context) ([]provider.Session, error) {
	db, err := o.open(true)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT
			s.id, s.title, s.directory, s.time_created, s.time_updated,
			p.worktree,
			(SELECT COUNT(*) FROM message WHERE session_id = s.id) as msg_count,
			(SELECT data FROM message
			 WHERE session_id = s.id AND json_extract(data, '$.role') = 'assistant'
			 ORDER BY time_created DESC LIMIT 1) as last_assistant,
			(SELECT data FROM message
			 WHERE session_id = s.id AND json_extract(data, '$.role') = 'user'
			 ORDER BY time_created ASC LIMIT 1) as first_user
		FROM session s
		LEFT JOIN project p ON s.project_id = p.id
		ORDER BY s.time_updated DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []provider.Session
	for rows.Next() {
		var (
			id, title, directory, worktree   sql.NullString
			timeCreated, timeUpdated         sql.NullInt64
			msgCount                         int
			lastAssistantData, firstUserData sql.NullString
		)
		if err := rows.Scan(&id, &title, &directory, &timeCreated, &timeUpdated,
			&worktree, &msgCount, &lastAssistantData, &firstUserData); err != nil {
			continue
		}

		projectPath := directory.String
		if projectPath == "" {
			projectPath = worktree.String
		}

		s := provider.Session{
			ID:           id.String,
			Provider:     "opencode",
			ProjectName:  filepath.Base(projectPath),
			ProjectPath:  projectPath,
			Summary:      title.String,
			Model:        extractModel(lastAssistantData),
			FirstPrompt:  extractPrompt(firstUserData),
			MessageCount: msgCount,
		}

		if timeCreated.Valid {
			s.Created = time.UnixMilli(timeCreated.Int64)
		}
		if timeUpdated.Valid {
			s.Modified = time.UnixMilli(timeUpdated.Int64)
		}

		sessions = append(sessions, s)
	}

	return sessions, nil
}

func (o *OpenCode) DeleteSession(sessionID string) error {
	db, err := o.open(false)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"part", "message", "todo", "session_diff"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE session_id = ?", sessionID); err != nil {
			return fmt.Errorf("deleting from %s: %w", table, err)
		}
	}
	if _, err := tx.Exec("DELETE FROM session WHERE id = ?", sessionID); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}

	return tx.Commit()
}

func (o *OpenCode) open(readOnly bool) (*sql.DB, error) {
	dsn := o.dbPath
	if readOnly {
		dsn += "?mode=ro"
	}
	return sql.Open("sqlite", dsn)
}

func extractModel(data sql.NullString) string {
	if !data.Valid {
		return ""
	}
	var msg struct {
		ProviderID string `json:"providerID"`
		ModelID    string `json:"modelID"`
		Model      struct {
			ProviderID string `json:"providerID"`
			ModelID    string `json:"modelID"`
		} `json:"model"`
	}
	if json.Unmarshal([]byte(data.String), &msg) != nil {
		return ""
	}
	pid := msg.ProviderID
	mid := msg.ModelID
	if mid == "" {
		pid = msg.Model.ProviderID
		mid = msg.Model.ModelID
	}
	if mid == "" {
		return ""
	}
	if pid != "" {
		return pid + "/" + mid
	}
	return mid
}

func extractPrompt(data sql.NullString) string {
	if !data.Valid {
		return ""
	}
	var msg struct {
		Content string `json:"content"`
	}
	if json.Unmarshal([]byte(data.String), &msg) == nil && msg.Content != "" {
		return msg.Content
	}
	return ""
}
