package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"vibecockpit/internal/provider"
)

type Claude struct{}

func New() *Claude { return &Claude{} }

func (c *Claude) Name() string { return "claude" }
func (c *Claude) Icon() string { return "●" }

func (c *Claude) ResumeCommand(s provider.Session) (string, []string) {
	args := []string{"--resume", s.ID}
	if s.Model != "" {
		args = append(args, "--model", s.Model)
	}
	return "claude", args
}

func (c *Claude) NewCommand(dir string) (string, []string) {
	return "claude", nil
}

func (c *Claude) DeleteSession(sessionID string) error {
	home, _ := os.UserHomeDir()
	projectsDir := filepath.Join(home, ".claude", "projects")

	projects, err := os.ReadDir(projectsDir)
	if err != nil {
		return err
	}

	for _, proj := range projects {
		if !proj.IsDir() {
			continue
		}
		projPath := filepath.Join(projectsDir, proj.Name())
		jsonlPath := filepath.Join(projPath, sessionID+".jsonl")

		if _, err := os.Stat(jsonlPath); err != nil {
			continue
		}

		_ = os.Remove(jsonlPath)
		_ = os.RemoveAll(filepath.Join(projPath, sessionID))

		indexPath := filepath.Join(projPath, "sessions-index.json")
		indexData, err := os.ReadFile(indexPath)
		if err == nil {
			var idx indexFile
			if json.Unmarshal(indexData, &idx) == nil {
				filtered := idx.Entries[:0]
				for _, e := range idx.Entries {
					if e.SessionID != sessionID {
						filtered = append(filtered, e)
					}
				}
				idx.Entries = filtered
				if data, err := json.MarshalIndent(idx, "", "  "); err == nil {
					_ = os.WriteFile(indexPath, data, 0644)
				}
			}
		}
		return nil
	}

	return fmt.Errorf("session not found: %s", sessionID)
}

func (c *Claude) ScanSessions(_ context.Context) ([]provider.Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	claudeDir := filepath.Join(home, ".claude")

	active := scanActivePIDs(claudeDir)
	sessions := scanProjects(claudeDir, active)

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})

	return sessions, nil
}

func scanActivePIDs(claudeDir string) map[string]int {
	result := make(map[string]int)
	entries, err := os.ReadDir(filepath.Join(claudeDir, "sessions"))
	if err != nil {
		return result
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(claudeDir, "sessions", e.Name()))
		if err != nil {
			continue
		}
		var as struct {
			PID       int    `json:"pid"`
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(data, &as) != nil || as.PID == 0 {
			continue
		}
		if syscall.Kill(as.PID, 0) == nil {
			result[as.SessionID] = as.PID
		}
	}
	return result
}

func scanProjects(claudeDir string, active map[string]int) []provider.Session {
	var sessions []provider.Session
	projectsDir := filepath.Join(claudeDir, "projects")

	projects, err := os.ReadDir(projectsDir)
	if err != nil {
		return sessions
	}

	for _, proj := range projects {
		if !proj.IsDir() {
			continue
		}
		projPath := filepath.Join(projectsDir, proj.Name())
		sessions = append(sessions, scanProject(projPath, proj.Name(), active)...)
	}
	return sessions
}

func decodeProjectDir(dirName string) string {
	if !strings.HasPrefix(dirName, "-") {
		return dirName
	}
	path := strings.ReplaceAll(dirName, "-", "/")
	return path
}

func scanProject(projPath string, dirName string, active map[string]int) []provider.Session {
	indexData, err := os.ReadFile(filepath.Join(projPath, "sessions-index.json"))
	if err != nil {
		return scanProjectFromJSONL(projPath, dirName, active)
	}

	var idx indexFile
	if json.Unmarshal(indexData, &idx) != nil {
		return scanProjectFromJSONL(projPath, dirName, active)
	}

	var sessions []provider.Session
	for _, entry := range idx.Entries {
		if entry.IsSidechain {
			continue
		}

		created, _ := time.Parse(time.RFC3339, entry.Created)
		modified, _ := time.Parse(time.RFC3339, entry.Modified)

		projectPath := entry.ProjectPath
		if projectPath == "" {
			projectPath = idx.OriginalPath
		}
		if projectPath == "" {
			projectPath = decodeProjectDir(dirName)
		}

		jsonlPath := filepath.Join(projPath, entry.SessionID+".jsonl")
		model := extractModelFromTail(jsonlPath)

		s := provider.Session{
			ID:           entry.SessionID,
			Provider:     "claude",
			ProjectName:  filepath.Base(projectPath),
			ProjectPath:  projectPath,
			Summary:      entry.Summary,
			FirstPrompt:  entry.FirstPrompt,
			Model:        model,
			GitBranch:    entry.GitBranch,
			MessageCount: entry.MessageCount,
			Modified:     modified,
			Created:      created,
			DataPath:     jsonlPath,
		}

		if pid, ok := active[entry.SessionID]; ok {
			s.IsActive = true
			s.ActivePID = pid
		}

		sessions = append(sessions, s)
	}
	return sessions
}

func scanProjectFromJSONL(projPath string, dirName string, active map[string]int) []provider.Session {
	entries, err := os.ReadDir(projPath)
	if err != nil {
		return nil
	}

	var sessions []provider.Session
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		jsonlPath := filepath.Join(projPath, e.Name())
		sessionID := strings.TrimSuffix(e.Name(), ".jsonl")

		fi, err := os.Stat(jsonlPath)
		if err != nil {
			continue
		}

		meta := extractMetadataFromJSONL(jsonlPath)
		model := extractModelFromTail(jsonlPath)

		projectPath := meta.cwd
		if projectPath == "" {
			projectPath = decodeProjectDir(dirName)
		}

		s := provider.Session{
			ID:           sessionID,
			Provider:     "claude",
			ProjectName:  filepath.Base(projectPath),
			ProjectPath:  projectPath,
			FirstPrompt:  meta.firstPrompt,
			Model:        model,
			GitBranch:    meta.gitBranch,
			MessageCount: countJSONLMessages(jsonlPath),
			Modified:     fi.ModTime(),
			Created:      meta.created,
			DataPath:     jsonlPath,
		}

		if pid, ok := active[sessionID]; ok {
			s.IsActive = true
			s.ActivePID = pid
		}

		sessions = append(sessions, s)
	}
	return sessions
}

func countJSONLMessages(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	userTag := []byte(`"type":"user"`)
	assistantTag := []byte(`"type":"assistant"`)
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.Contains(line, userTag) || bytes.Contains(line, assistantTag) {
			count++
		}
	}
	return count
}

func extractModelFromTail(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return ""
	}

	readSize := int64(32768)
	offset := fi.Size() - readSize
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}

	data, _ := io.ReadAll(f)
	lines := bytes.Split(data, []byte("\n"))

	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if !bytes.Contains(line, []byte(`"assistant"`)) || !bytes.Contains(line, []byte(`"model"`)) {
			continue
		}
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) == nil && entry.Type == "assistant" && entry.Message.Model != "" {
			if strings.HasPrefix(entry.Message.Model, "claude-") {
				return entry.Message.Model
			}
		}
	}
	return ""
}

type jsonlMeta struct {
	cwd         string
	gitBranch   string
	firstPrompt string
	created     time.Time
}

func extractMetadataFromJSONL(path string) jsonlMeta {
	f, err := os.Open(path)
	if err != nil {
		return jsonlMeta{}
	}
	defer f.Close()

	var meta jsonlMeta
	buf := make([]byte, 65536)
	n, _ := f.Read(buf)
	lines := bytes.Split(buf[:n], []byte("\n"))

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var entry struct {
			Type      string `json:"type"`
			CWD       string `json:"cwd"`
			GitBranch string `json:"gitBranch"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil {
			continue
		}
		if entry.Type != "user" {
			continue
		}

		meta.cwd = entry.CWD
		meta.gitBranch = entry.GitBranch
		if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
			meta.created = t
		}

		var text string
		if err := json.Unmarshal(entry.Message.Content, &text); err == nil {
			meta.firstPrompt = text
		} else {
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(entry.Message.Content, &blocks) == nil {
				for _, b := range blocks {
					if b.Type == "text" {
						text += b.Text
					}
				}
				meta.firstPrompt = text
			}
		}
		break
	}
	return meta
}

type indexFile struct {
	Version      int          `json:"version"`
	OriginalPath string       `json:"originalPath"`
	Entries      []indexEntry `json:"entries"`
}

type indexEntry struct {
	SessionID    string `json:"sessionId"`
	FullPath     string `json:"fullPath"`
	FileMtime    int64  `json:"fileMtime"`
	FirstPrompt  string `json:"firstPrompt"`
	Summary      string `json:"summary"`
	MessageCount int    `json:"messageCount"`
	Created      string `json:"created"`
	Modified     string `json:"modified"`
	GitBranch    string `json:"gitBranch"`
	ProjectPath  string `json:"projectPath"`
	IsSidechain  bool   `json:"isSidechain"`
}
