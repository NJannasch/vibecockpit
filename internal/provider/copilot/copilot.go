package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vibecockpit/internal/provider"

	"gopkg.in/yaml.v3"
)

type Copilot struct {
	baseDir string
}

func New() *Copilot {
	home, _ := os.UserHomeDir()
	return &Copilot{
		baseDir: filepath.Join(home, ".copilot"),
	}
}

func (c *Copilot) Available() bool {
	_, err := os.Stat(filepath.Join(c.baseDir, "session-state"))
	return err == nil
}

func (c *Copilot) Name() string { return "copilot" }
func (c *Copilot) Icon() string { return "●" }

func (c *Copilot) ResumeCommand(s provider.Session) (string, []string) {
	return "copilot", []string{"--resume=" + s.ID}
}

func (c *Copilot) NewCommand(dir string) (string, []string) {
	return "copilot", nil
}

func (c *Copilot) ScanSessions(_ context.Context) ([]provider.Session, error) {
	sessDir := filepath.Join(c.baseDir, "session-state")
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		return nil, err
	}

	var sessions []provider.Session
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(sessDir, e.Name())
		s, err := c.scanSession(dir)
		if err != nil {
			continue
		}
		sessions = append(sessions, *s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})

	return sessions, nil
}

func (c *Copilot) DeleteSession(sessionID string) error {
	dir := filepath.Join(c.baseDir, "session-state", sessionID)
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

type workspace struct {
	ID        string `yaml:"id"`
	CWD       string `yaml:"cwd"`
	Summary   string `yaml:"summary"`
	CreatedAt string `yaml:"created_at"`
	UpdatedAt string `yaml:"updated_at"`
}

func (c *Copilot) scanSession(dir string) (*provider.Session, error) {
	data, err := os.ReadFile(filepath.Join(dir, "workspace.yaml"))
	if err != nil {
		return nil, err
	}

	var ws workspace
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return nil, err
	}

	created, _ := time.Parse(time.RFC3339Nano, ws.CreatedAt)
	modified, _ := time.Parse(time.RFC3339Nano, ws.UpdatedAt)

	model, firstPrompt, msgCount := scanEvents(filepath.Join(dir, "events.jsonl"))

	return &provider.Session{
		ID:           ws.ID,
		Provider:     "copilot",
		ProjectName:  filepath.Base(ws.CWD),
		ProjectPath:  ws.CWD,
		Summary:      ws.Summary,
		FirstPrompt:  firstPrompt,
		Model:        model,
		MessageCount: msgCount,
		Created:      created,
		Modified:     modified,
	}, nil
}

type event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func scanEvents(path string) (model, firstPrompt string, msgCount int) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		var ev event
		if json.Unmarshal(scanner.Bytes(), &ev) != nil {
			continue
		}

		switch ev.Type {
		case "session.model_change":
			var d struct {
				NewModel string `json:"newModel"`
			}
			json.Unmarshal(ev.Data, &d)
			model = d.NewModel

		case "user.message":
			msgCount++
			if firstPrompt == "" {
				var d struct {
					Content string `json:"content"`
				}
				json.Unmarshal(ev.Data, &d)
				firstPrompt = strings.TrimSpace(d.Content)
			}

		case "assistant.turn_start":
			msgCount++
		}
	}

	return
}
