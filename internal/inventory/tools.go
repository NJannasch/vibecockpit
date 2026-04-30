package inventory

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type toolDef struct {
	ID        string
	Name      string
	Binary    string
	ConfigDir string // relative to home
	DataDir   string // relative to home
	VersionFlag string
}

var toolDefs = []toolDef{
	{
		ID: "claude", Name: "Claude Code", Binary: "claude",
		ConfigDir: ".claude", DataDir: ".claude/projects",
		VersionFlag: "--version",
	},
	{
		ID: "codex", Name: "Codex CLI", Binary: "codex",
		ConfigDir: ".codex", DataDir: ".codex",
		VersionFlag: "--version",
	},
	{
		ID: "copilot", Name: "Copilot CLI", Binary: "github-copilot",
		ConfigDir: ".copilot", DataDir: ".copilot/session-state",
		VersionFlag: "--version",
	},
	{
		ID: "gemini", Name: "Gemini CLI", Binary: "gemini",
		ConfigDir: ".gemini", DataDir: ".gemini/tmp",
		VersionFlag: "--version",
	},
	{
		ID: "opencode", Name: "OpenCode", Binary: "opencode",
		ConfigDir: ".config/opencode", DataDir: ".local/share/opencode",
		VersionFlag: "--version",
	},
	{
		ID: "cursor-cli", Name: "Cursor Agent (CLI)", Binary: "agent",
		ConfigDir: ".cursor", DataDir: ".cursor/chats",
		VersionFlag: "--version",
	},
	{
		ID: "cursor-ide", Name: "Cursor (IDE)", Binary: "cursor",
		ConfigDir: ".config/Cursor", DataDir: ".config/Cursor/User/globalStorage",
		VersionFlag: "--version",
	},
	{
		ID: "antigravity", Name: "Antigravity", Binary: "antigravity",
		ConfigDir: ".gemini/antigravity", DataDir: ".gemini/antigravity/conversations",
		VersionFlag: "--version",
	},
	{
		ID: "aider", Name: "Aider", Binary: "aider",
		VersionFlag: "--version",
	},
	{
		ID: "goose", Name: "Goose", Binary: "goose",
		ConfigDir: ".config/goose",
		VersionFlag: "--version",
	},
	{
		ID: "amp", Name: "Amp", Binary: "amp",
		ConfigDir: ".config/amp",
		VersionFlag: "--version",
	},
	{
		ID: "devin", Name: "Devin", Binary: "devin",
		ConfigDir: ".config/devin",
		VersionFlag: "--version",
	},
	{
		ID: "amazonq", Name: "Amazon Q Developer", Binary: "q",
		ConfigDir: ".aws/amazonq",
		VersionFlag: "--version",
	},
	{
		ID: "continue", Name: "Continue.dev", Binary: "continue",
		ConfigDir: ".continue",
	},
	{
		ID: "windsurf", Name: "Windsurf", Binary: "windsurf",
		ConfigDir: ".codeium/windsurf",
	},
	{
		ID: "trae", Name: "Trae", Binary: "trae",
	},
	{
		ID: "kilo", Name: "Kilo Code", Binary: "kilo",
		ConfigDir: ".config/kilo",
	},
	{
		ID: "qwen", Name: "Qwen Code", Binary: "qwen",
		ConfigDir: ".qwen",
	},
}

func init() {
	if runtime.GOOS == "darwin" {
		for i := range toolDefs {
			if toolDefs[i].ID == "cursor-ide" {
				toolDefs[i].ConfigDir = "Library/Application Support/Cursor"
				toolDefs[i].DataDir = "Library/Application Support/Cursor/User/globalStorage"
			}
		}
	}
}

func scanTools() []ToolInfo {
	home, _ := os.UserHomeDir()
	tools := make([]ToolInfo, 0, len(toolDefs))

	for _, def := range toolDefs {
		t := ToolInfo{
			ID:   def.ID,
			Name: def.Name,
		}

		if bin, err := exec.LookPath(def.Binary); err == nil {
			t.Installed = true
			t.BinaryPath = bin
			t.Version = getVersion(bin, def.VersionFlag)
		}

		if def.ConfigDir != "" {
			dir := filepath.Join(home, def.ConfigDir)
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				t.ConfigDir = dir
				if !t.Installed {
					t.Installed = true
				}
			}
		}

		if def.DataDir != "" {
			dir := filepath.Join(home, def.DataDir)
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				t.DataDir = dir
			}
		}

		tools = append(tools, t)
	}

	return tools
}

func getVersion(bin, flag string) string {
	if flag == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, flag).CombinedOutput()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	// Take first line only
	if i := strings.IndexByte(v, '\n'); i > 0 {
		v = v[:i]
	}
	if len(v) > 100 {
		v = v[:100]
	}
	return v
}
