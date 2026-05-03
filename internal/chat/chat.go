package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vibecockpit/internal/config"
	"vibecockpit/internal/runner"
)

type Message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Tool      string `json:"tool,omitempty"`
	Model     string `json:"model,omitempty"`
}

type Chat struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Tool       string   `json:"tool"`
	Model      string   `json:"model"`
	Project    string   `json:"project,omitempty"`
	MCPServers []string `json:"mcpServers,omitempty"`
	Messages   []Message `json:"messages"`
	CreatedAt  string   `json:"createdAt"`
	UpdatedAt  string   `json:"updatedAt"`
}

type SendOpts struct {
	Content    string   `json:"content"`
	Tool       string   `json:"tool,omitempty"`
	Model      string   `json:"model,omitempty"`
	MCPServers []string `json:"mcpServers,omitempty"`
}

type Manager struct {
	mu       sync.Mutex
	chatsDir string
	cfg      *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		chatsDir: chatsDir(),
		cfg:      cfg,
	}
}

func chatsDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "vibecockpit", "chats")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vibecockpit", "chats")
}

func (m *Manager) ListChats() ([]Chat, error) {
	entries, err := os.ReadDir(m.chatsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Chat{}, nil
		}
		return nil, err
	}

	var chats []Chat
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c, err := m.loadChat(e.Name())
		if err != nil {
			continue
		}
		chats = append(chats, *c)
	}
	return chats, nil
}

func (m *Manager) GetChat(id string) (*Chat, error) {
	return m.loadChat(id)
}

func (m *Manager) CreateChat(name, tool, model, project string, mcpServers []string) (*Chat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := generateID(name)
	chatDir := filepath.Join(m.chatsDir, id)

	if _, err := os.Stat(chatDir); err == nil {
		id = fmt.Sprintf("%s-%d", id, time.Now().Unix())
		chatDir = filepath.Join(m.chatsDir, id)
	}

	if err := os.MkdirAll(chatDir, 0755); err != nil {
		return nil, err
	}

	if tool == "" {
		tool = "claude"
	}
	if mcpServers == nil {
		mcpServers = []string{"vibecockpit"}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c := &Chat{
		ID:         id,
		Name:       name,
		Tool:       tool,
		Model:      model,
		Project:    project,
		MCPServers: mcpServers,
		Messages:   []Message{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := m.ensureMCP(chatDir, tool); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write MCP config for chat: %v\n", err)
	}

	return c, m.saveChat(c)
}

func (m *Manager) DeleteChat(id string) error {
	chatDir := filepath.Join(m.chatsDir, id)
	if _, err := os.Stat(chatDir); os.IsNotExist(err) {
		return fmt.Errorf("chat %q not found", id)
	}
	return os.RemoveAll(chatDir)
}

func (m *Manager) SendMessage(id string, opts SendOpts) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, err := m.loadChat(id)
	if err != nil {
		return nil, err
	}

	tool := opts.Tool
	if tool == "" {
		tool = c.Tool
	}
	model := opts.Model
	if model == "" {
		model = c.Model
	}

	now := time.Now().UTC().Format(time.RFC3339)
	userMsg := Message{Role: "user", Content: opts.Content, Timestamp: now, Tool: tool, Model: model}
	c.Messages = append(c.Messages, userMsg)

	chatDir := filepath.Join(m.chatsDir, id)

	mcpServers := opts.MCPServers
	if mcpServers == nil {
		mcpServers = c.MCPServers
	}
	if err := m.ensureMCPWithServers(chatDir, tool, mcpServers); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write MCP config for chat: %v\n", err)
	}

	prompt := m.buildPrompt(c)
	response, err := m.runAgent(chatDir, tool, model, prompt)
	if err != nil {
		c.UpdatedAt = now
		_ = m.saveChat(c)
		return nil, fmt.Errorf("agent error: %w", err)
	}

	assistantMsg := Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Tool:      tool,
		Model:     model,
	}
	c.Messages = append(c.Messages, assistantMsg)
	c.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := m.saveChat(c); err != nil {
		return nil, err
	}
	return &assistantMsg, nil
}

func (m *Manager) buildPrompt(c *Chat) string {
	var parts []string

	parts = append(parts, "You are in an interactive chat session. Format your responses in Markdown. You have access to VibeCockpit MCP tools for managing boards, tasks, scheduled jobs, sessions, and costs. Use them when the user asks you to create tickets, schedule jobs, check status, etc. You can also create and edit files in the working directory.\n")

	if c.Project != "" {
		parts = append(parts, fmt.Sprintf("Working directory: %s\n", c.Project))
	}

	if len(c.Messages) > 1 {
		parts = append(parts, "Conversation so far:")
		limit := len(c.Messages)
		if limit > 20 {
			limit = 20
			parts = append(parts, fmt.Sprintf("(showing last %d messages)\n", limit))
		}
		for _, msg := range c.Messages[len(c.Messages)-limit:] {
			prefix := "User"
			if msg.Role == "assistant" {
				prefix = "Assistant"
			}
			content := msg.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			parts = append(parts, fmt.Sprintf("%s: %s", prefix, content))
		}
		parts = append(parts, "\nRespond to the latest user message above.")
	} else {
		parts = append(parts, "User: "+c.Messages[len(c.Messages)-1].Content)
		parts = append(parts, "\nRespond to the user's message.")
	}

	return strings.Join(parts, "\n")
}

func (m *Manager) runAgent(chatDir, tool, model, prompt string) (string, error) {
	tc := runner.ToolConfigFor(tool)
	binPath := runner.ResolveBin(m.cfg, tool, tc.Bin)
	if binPath == "" {
		return "", fmt.Errorf("could not find %q in PATH", tc.Bin)
	}

	args := runner.BuildArgs(tc, model, prompt)

	cmd := exec.Command(binPath, args...)
	cmd.Dir = chatDir
	cmd.Env = runner.BuildEnvForChat(m.cfg, binPath)

	out, err := cmd.CombinedOutput()
	response := strings.TrimSpace(string(out))

	if err != nil {
		if response != "" {
			return response, nil
		}
		return "", err
	}
	return response, nil
}

func (m *Manager) ensureMCP(chatDir, tool string) error {
	tc := runner.ToolConfigFor(tool)
	return runner.EnsureMCPConfigPublic(chatDir, tc)
}

func (m *Manager) ensureMCPWithServers(chatDir, tool string, _ []string) error {
	tc := runner.ToolConfigFor(tool)
	return runner.EnsureMCPConfigPublic(chatDir, tc)
}

func (m *Manager) loadChat(id string) (*Chat, error) {
	path := filepath.Join(m.chatsDir, id, "chat.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Chat
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (m *Manager) saveChat(c *Chat) error {
	dir := filepath.Join(m.chatsDir, c.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "chat.json"), data, 0644)
}

type ChatFile struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	IsDir    bool   `json:"isDir"`
}

func (m *Manager) ListFiles(id string) ([]ChatFile, error) {
	chatDir := filepath.Join(m.chatsDir, id)
	if _, err := os.Stat(chatDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("chat %q not found", id)
	}

	var files []ChatFile
	entries, err := os.ReadDir(chatDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "chat.json" || name == ".mcp.json" || strings.HasPrefix(name, ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, ChatFile{
			Name:     name,
			Path:     filepath.Join(chatDir, name),
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
			IsDir:    e.IsDir(),
		})
	}
	return files, nil
}

func (m *Manager) ReadFile(id, filename string) (string, error) {
	chatDir := filepath.Join(m.chatsDir, id)
	path := filepath.Join(chatDir, filepath.Clean(filename))
	if !strings.HasPrefix(path, chatDir) {
		return "", fmt.Errorf("invalid path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > 512*1024 {
		data = data[:512*1024]
	}
	return string(data), nil
}

func generateID(name string) string {
	id := strings.ToLower(name)
	id = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, id)
	for strings.Contains(id, "--") {
		id = strings.ReplaceAll(id, "--", "-")
	}
	id = strings.Trim(id, "-")
	if len(id) > 30 {
		id = id[:30]
	}
	if id == "" {
		id = "chat"
	}
	return id
}
