package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"vibecockpit/internal/plugin"
	"vibecockpit/internal/provider"
)

type RemotePlugin struct {
	meta    plugin.Metadata
	cfg     RemoteConfig
	enabled bool
}

type RemoteConfig struct {
	Name   string `json:"name" yaml:"name"`
	Host   string `json:"host" yaml:"host"`
	User   string `json:"user" yaml:"user"`
	Port   int    `json:"port" yaml:"port"`
	Method string `json:"method" yaml:"method"` // "ssh" or "http"
	URL    string `json:"url" yaml:"url"`
}

func New(id, name string) *RemotePlugin {
	return &RemotePlugin{
		meta: plugin.Metadata{
			ID:          id,
			Name:        name,
			Type:        "remote",
			Icon:        "⬡",
			Version:     "1.0.0",
			License:     "free",
			Description: "Scan sessions on a remote machine",
		},
	}
}

func (p *RemotePlugin) Metadata() plugin.Metadata  { return p.meta }
func (p *RemotePlugin) Enabled() bool               { return p.enabled }
func (p *RemotePlugin) SetEnabled(e bool)            { p.enabled = e }
func (p *RemotePlugin) Provider() provider.Provider  { return &remoteProvider{plugin: p} }

func (p *RemotePlugin) Init(cfg map[string]any) error {
	data, _ := json.Marshal(cfg)
	var rc RemoteConfig
	if err := json.Unmarshal(data, &rc); err != nil {
		return fmt.Errorf("invalid remote config: %w", err)
	}
	if rc.Host == "" && rc.URL == "" {
		p.enabled = false
		return nil
	}
	if rc.Method == "" {
		rc.Method = "ssh"
	}
	if rc.Port == 0 {
		rc.Port = 22
	}
	p.cfg = rc
	if p.cfg.Name != "" {
		p.meta.Name = p.cfg.Name
	}
	p.enabled = true
	return nil
}

type remoteProvider struct {
	plugin *RemotePlugin
}

func (r *remoteProvider) Name() string { return r.plugin.meta.ID }
func (r *remoteProvider) Icon() string { return r.plugin.meta.Icon }

func (r *remoteProvider) ScanSessions(ctx context.Context) ([]provider.Session, error) {
	switch r.plugin.cfg.Method {
	case "ssh":
		return r.scanSSH(ctx)
	case "http":
		return r.scanHTTP(ctx)
	default:
		return nil, fmt.Errorf("unsupported method: %s", r.plugin.cfg.Method)
	}
}

var resumeCommands = map[string]func(s provider.Session) string{
	"claude":  func(s provider.Session) string { return fmt.Sprintf("claude --resume %s", s.ID) },
	"codex":   func(s provider.Session) string { return fmt.Sprintf("codex resume %s", s.ID) },
	"copilot": func(s provider.Session) string { return fmt.Sprintf("copilot --resume=%s", s.ID) },
	"hermes":  func(s provider.Session) string { return "hermes" },
}

func (r *remoteProvider) ResumeCommand(s provider.Session) (string, []string) {
	cfg := r.plugin.cfg

	// Use DataPath to store the original tool name from remote scanning
	tool := s.DataPath
	if tool == "" {
		tool = "claude"
	}
	resumeFn, ok := resumeCommands[tool]
	var remoteCmd string
	if ok {
		remoteCmd = resumeFn(s)
	} else {
		remoteCmd = tool
	}
	if s.ProjectPath != "" && s.ProjectPath != "~/.hermes" {
		remoteCmd = fmt.Sprintf("cd %s && %s", s.ProjectPath, remoteCmd)
	}

	return "ssh", []string{
		"-t",
		fmt.Sprintf("%s@%s", cfg.User, cfg.Host),
		"-p", fmt.Sprintf("%d", cfg.Port),
		remoteCmd,
	}
}

func (r *remoteProvider) NewCommand(dir string) (string, []string) {
	cfg := r.plugin.cfg
	return "ssh", []string{
		"-t",
		fmt.Sprintf("%s@%s", cfg.User, cfg.Host),
		"-p", fmt.Sprintf("%d", cfg.Port),
		fmt.Sprintf("cd %s && claude", dir),
	}
}

func (r *remoteProvider) DeleteSession(_ string) error {
	return fmt.Errorf("delete not supported for remote sessions")
}

func (r *remoteProvider) scanSSH(ctx context.Context) ([]provider.Session, error) {
	source := r.plugin.meta.ID

	// First try: vibecockpit --list --json (if installed on remote)
	sessions, err := r.tryVibecockpitSSH(ctx)
	if err == nil && len(sessions) > 0 {
		for i := range sessions {
			sessions[i].Provider = source
		}
		return sessions, nil
	}

	// Fallback: scan remote session files directly
	return r.scanRemoteFiles(ctx, source)
}

func (r *remoteProvider) tryVibecockpitSSH(ctx context.Context) ([]provider.Session, error) {
	out, err := sshCmd(ctx, r.plugin.cfg, "vibecockpit --list --json 2>/dev/null")
	if err != nil {
		return nil, err
	}
	var sessions []provider.Session
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *remoteProvider) scanRemoteFiles(ctx context.Context, source string) ([]provider.Session, error) {
	cfg := r.plugin.cfg
	var all []provider.Session

	// Scan Claude Code sessions-index.json files
	claudeSessions, _ := r.scanClaudeSSH(ctx, cfg, source)
	all = append(all, claudeSessions...)

	// Scan Hermes sessions
	hermesSessions, _ := r.scanHermesSSH(ctx, cfg, source)
	all = append(all, hermesSessions...)

	// Scan Codex state DB (extract via sqlite3 on remote if available)
	codexSessions, _ := r.scanCodexSSH(ctx, cfg, source)
	all = append(all, codexSessions...)

	// Scan Copilot CLI workspace.yaml files
	copilotSessions, _ := r.scanCopilotSSH(ctx, cfg, source)
	all = append(all, copilotSessions...)

	// Scan Gemini CLI session JSON files
	geminiSessions, _ := r.scanGeminiSSH(ctx, cfg, source)
	all = append(all, geminiSessions...)

	return all, nil
}

func (r *remoteProvider) scanClaudeSSH(ctx context.Context, cfg RemoteConfig, source string) ([]provider.Session, error) {
	// Find and cat all sessions-index.json files
	script := `find ~/.claude/projects -name 'sessions-index.json' -exec cat {} \; 2>/dev/null`
	out, err := sshCmd(ctx, cfg, script)
	if err != nil || len(out) == 0 {
		return nil, err
	}

	var sessions []provider.Session
	// Each sessions-index.json is a separate JSON object, parse them
	decoder := json.NewDecoder(bytes.NewReader(out))
	for decoder.More() {
		var idx struct {
			OriginalPath string `json:"originalPath"`
			Entries      []struct {
				SessionID    string `json:"sessionId"`
				Summary      string `json:"summary"`
				FirstPrompt  string `json:"firstPrompt"`
				MessageCount int    `json:"messageCount"`
				Created      string `json:"created"`
				Modified     string `json:"modified"`
				GitBranch    string `json:"gitBranch"`
				ProjectPath  string `json:"projectPath"`
				IsSidechain  bool   `json:"isSidechain"`
			} `json:"entries"`
		}
		if err := decoder.Decode(&idx); err != nil {
			break
		}
		for _, e := range idx.Entries {
			if e.IsSidechain {
				continue
			}
			created, _ := time.Parse(time.RFC3339, e.Created)
			modified, _ := time.Parse(time.RFC3339, e.Modified)
			path := e.ProjectPath
			if path == "" {
				path = idx.OriginalPath
			}
			sessions = append(sessions, provider.Session{
				ID:           e.SessionID,
				Provider:     source,
				DataPath:  "claude",
				ProjectName:  baseName(path),
				ProjectPath:  path,
				Summary:      e.Summary,
				FirstPrompt:  e.FirstPrompt,
				GitBranch:    e.GitBranch,
				MessageCount: e.MessageCount,
				Created:      created,
				Modified:     modified,
			})
		}
	}
	return sessions, nil
}

func (r *remoteProvider) scanHermesSSH(ctx context.Context, cfg RemoteConfig, source string) ([]provider.Session, error) {
	out, err := sshCmd(ctx, cfg, `cat ~/.hermes/sessions/sessions.json 2>/dev/null`)
	if err != nil || len(out) == 0 {
		return nil, err
	}

	var index map[string]struct {
		SessionID   string `json:"session_id"`
		DisplayName string `json:"display_name"`
		Platform    string `json:"platform"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	}
	if err := json.Unmarshal(out, &index); err != nil {
		return nil, err
	}

	var sessions []provider.Session
	for _, entry := range index {
		created, _ := time.Parse("2006-01-02T15:04:05.999999", entry.CreatedAt)
		modified, _ := time.Parse("2006-01-02T15:04:05.999999", entry.UpdatedAt)

		sessions = append(sessions, provider.Session{
			ID:          entry.SessionID,
			Provider:    source,
			DataPath:    "hermes",
			ProjectName: "hermes",
			ProjectPath: "~/.hermes",
			Summary:     fmt.Sprintf("%s via %s", entry.DisplayName, entry.Platform),
			Model:       "hermes",
			Created:     created,
			Modified:    modified,
		})
	}

	// Also count JSONL lines for message counts
	countOut, _ := sshCmd(ctx, cfg, `for f in ~/.hermes/sessions/*.jsonl; do echo "$(basename "$f" .jsonl) $(wc -l < "$f")"; done 2>/dev/null`)
	counts := make(map[string]int)
	for _, line := range strings.Split(string(countOut), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 {
			var n int
			_, _ = fmt.Sscanf(parts[1], "%d", &n)
			counts[parts[0]] = n
		}
	}
	for i := range sessions {
		if n, ok := counts[sessions[i].ID]; ok {
			sessions[i].MessageCount = n
		}
	}

	return sessions, nil
}

func (r *remoteProvider) scanCodexSSH(ctx context.Context, cfg RemoteConfig, source string) ([]provider.Session, error) {
	script := `sqlite3 -json ~/.codex/state_5.sqlite "SELECT id,title,cwd,model_provider,model,created_at,updated_at,tokens_used,first_user_message,git_branch,archived FROM threads WHERE archived=0 ORDER BY updated_at DESC" 2>/dev/null`
	out, err := sshCmd(ctx, cfg, script)
	if err != nil || len(out) == 0 {
		return nil, err
	}

	var rows []struct {
		ID               string `json:"id"`
		Title            string `json:"title"`
		CWD              string `json:"cwd"`
		ModelProvider    string `json:"model_provider"`
		Model            string `json:"model"`
		CreatedAt        int64  `json:"created_at"`
		UpdatedAt        int64  `json:"updated_at"`
		FirstUserMessage string `json:"first_user_message"`
		GitBranch        string `json:"git_branch"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, err
	}

	var sessions []provider.Session
	for _, row := range rows {
		model := row.Model
		if model == "" {
			model = row.ModelProvider
		}
		if row.ModelProvider != "" && row.Model != "" {
			model = row.ModelProvider + "/" + row.Model
		}
		sessions = append(sessions, provider.Session{
			ID:          row.ID,
			Provider:    source,
			DataPath:    "codex",
			ProjectName: baseName(row.CWD),
			ProjectPath: row.CWD,
			Summary:     row.Title,
			FirstPrompt: row.FirstUserMessage,
			Model:       model,
			GitBranch:   row.GitBranch,
			Created:     time.Unix(row.CreatedAt, 0),
			Modified:    time.Unix(row.UpdatedAt, 0),
		})
	}
	return sessions, nil
}

func (r *remoteProvider) scanHTTP(ctx context.Context) ([]provider.Session, error) {
	cfg := r.plugin.cfg
	url := strings.TrimRight(cfg.URL, "/") + "/api/sessions"

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "curl", "-sf", url).Output()
	if err != nil {
		return nil, fmt.Errorf("http fetch from %s failed: %w", url, err)
	}

	var sessions []provider.Session
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, err
	}

	source := r.plugin.meta.ID
	for i := range sessions {
		sessions[i].Provider = source
	}
	return sessions, nil
}

func (r *remoteProvider) scanCopilotSSH(ctx context.Context, cfg RemoteConfig, source string) ([]provider.Session, error) {
	script := `for d in ~/.copilot/session-state/*/; do [ -f "$d/workspace.yaml" ] && echo "---START---" && cat "$d/workspace.yaml"; done 2>/dev/null`
	out, err := sshCmd(ctx, cfg, script)
	if err != nil || len(out) == 0 {
		return nil, err
	}

	var sessions []provider.Session
	for _, block := range strings.Split(string(out), "---START---") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var id, cwd, summary, created, updated string
		for _, line := range strings.Split(block, "\n") {
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) != 2 {
				continue
			}
			switch strings.TrimSpace(parts[0]) {
			case "id":
				id = strings.TrimSpace(parts[1])
			case "cwd":
				cwd = strings.TrimSpace(parts[1])
			case "summary":
				summary = strings.TrimSpace(parts[1])
			case "created_at":
				created = strings.TrimSpace(parts[1])
			case "updated_at":
				updated = strings.TrimSpace(parts[1])
			}
		}
		if id == "" {
			continue
		}
		createdT, _ := time.Parse(time.RFC3339Nano, created)
		updatedT, _ := time.Parse(time.RFC3339Nano, updated)
		sessions = append(sessions, provider.Session{
			ID:          id,
			Provider:    source,
			DataPath:    "copilot",
			ProjectName: baseName(cwd),
			ProjectPath: cwd,
			Summary:     summary,
			Created:     createdT,
			Modified:    updatedT,
		})
	}
	return sessions, nil
}

func (r *remoteProvider) scanGeminiSSH(ctx context.Context, cfg RemoteConfig, source string) ([]provider.Session, error) {
	script := `find ~/.gemini/tmp -path '*/chats/*.json' -exec cat {} \; 2>/dev/null | head -c 500000`
	out, err := sshCmd(ctx, cfg, script)
	if err != nil || len(out) == 0 {
		return nil, err
	}

	var sessions []provider.Session
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var sf struct {
			SessionID   string `json:"sessionId"`
			StartTime   string `json:"startTime"`
			LastUpdated string `json:"lastUpdated"`
			Messages    []struct {
				Type    string `json:"type"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := decoder.Decode(&sf); err != nil {
			break
		}
		if sf.SessionID == "" {
			continue
		}
		created, _ := time.Parse(time.RFC3339Nano, sf.StartTime)
		modified, _ := time.Parse(time.RFC3339Nano, sf.LastUpdated)

		var firstPrompt string
		msgCount := 0
		for _, m := range sf.Messages {
			if m.Type == "user" || m.Type == "gemini" {
				msgCount++
			}
			if m.Type == "user" && firstPrompt == "" {
				firstPrompt = m.Content
				if len(firstPrompt) > 100 {
					firstPrompt = firstPrompt[:100]
				}
			}
		}

		sessions = append(sessions, provider.Session{
			ID:           sf.SessionID,
			Provider:     source,
			DataPath:     "gemini",
			ProjectName:  "gemini-session",
			Summary:      firstPrompt,
			FirstPrompt:  firstPrompt,
			Model:        "gemini",
			MessageCount: msgCount,
			Created:      created,
			Modified:     modified,
		})
	}
	return sessions, nil
}

func sshCmd(ctx context.Context, cfg RemoteConfig, command string) ([]byte, error) {
	args := []string{
		fmt.Sprintf("%s@%s", cfg.User, cfg.Host),
		"-p", fmt.Sprintf("%d", cfg.Port),
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		command,
	}
	return exec.CommandContext(ctx, "ssh", args...).Output()
}

func baseName(path string) string {
	if path == "" {
		return ""
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
