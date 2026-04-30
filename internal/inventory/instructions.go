package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

var knownInstructionFiles = []struct {
	Type string
	Path string // relative to project root
}{
	// Claude Code
	{"CLAUDE.md", "CLAUDE.md"},
	{"CLAUDE.local.md", "CLAUDE.local.md"},
	// Cursor
	{".cursorrules", ".cursorrules"},
	{".cursorignore", ".cursorignore"},
	{".cursorindexingignore", ".cursorindexingignore"},
	// GitHub Copilot
	{"copilot-instructions.md", ".github/copilot-instructions.md"},
	// Gemini
	{"GEMINI.md", "GEMINI.md"},
	{".geminiignore", ".geminiignore"},
	// Generic agent instructions
	{"agents.md", "agents.md"},
	{"AGENTS.md", "AGENTS.md"},
	{"AGENTS.override.md", "AGENTS.override.md"},
	{"CONVENTIONS.md", "CONVENTIONS.md"},
	// Codex
	{"codex.md", "codex.md"},
	{"CODEX.md", "CODEX.md"},
	{".codexrc", ".codexrc"},
	{".codex/config.toml", ".codex/config.toml"},
	// OpenCode
	{".opencode", ".opencode"},
	{"opencode.json", "opencode.json"},
	// Windsurf
	{".windsurfrules", ".windsurfrules"},
	// Junie (JetBrains)
	{".junie/AGENTS.md", ".junie/AGENTS.md"},
	{".aiignore", ".aiignore"},
	// Aider
	{".aider.conf.yml", ".aider.conf.yml"},
	{".aiderignore", ".aiderignore"},
	{".aider.model.metadata.json", ".aider.model.metadata.json"},
	// Roo Code / Kilo Code
	{".rooignore", ".rooignore"},
	{".roomodes", ".roomodes"},
	// Zed
	{".zed/settings.json", ".zed/settings.json"},
	// Augment Code
	{".augment-guidelines", ".augment-guidelines"},
	{".augmentignore", ".augmentignore"},
	// Goose
	{".goosehints", ".goosehints"},
	// Void
	{".voidrules", ".voidrules"},
	// Trae (ByteDance)
	{".trae/rules", ".trae/rules"},
	// Tabnine
	{".tabnineignore", ".tabnineignore"},
	// Qwen Code
	{".qwenignore", ".qwenignore"},
	// Cody
	{".cody/ignore", ".cody/ignore"},
	// Continue.dev
	{".continuerc.json", ".continuerc.json"},
	// Amazon Q
	{".amazonq/mcp.json", ".amazonq/mcp.json"},
	// VS Code MCP
	{".vscode/mcp.json", ".vscode/mcp.json"},
	// Devin
	{".devin/skills", ".devin/skills"},
}

var globalInstructionFiles = []struct {
	Type string
	Path string // relative to home dir
}{
	{"CLAUDE.md (global)", ".claude/CLAUDE.md"},
	{"GEMINI.md (global)", ".gemini/GEMINI.md"},
	{"copilot-instructions.md (global)", ".copilot/copilot-instructions.md"},
	{"AGENTS.md (global)", ".codex/AGENTS.md"},
	{".goosehints (global)", ".config/goose/.goosehints"},
	{".aider.conf.yml (global)", ".aider.conf.yml"},
	{".aider.conventions.md (global)", ".aider.conventions.md"},
}

func scanInstructionFiles(projectPaths []string) []InstructionFile {
	var files []InstructionFile

	home, _ := os.UserHomeDir()
	for _, def := range globalInstructionFiles {
		path := filepath.Join(home, def.Path)
		info, err := os.Stat(path)
		found := err == nil && !info.IsDir()
		logScan(path, "instruction-file", found)
		if found {
			files = append(files, InstructionFile{
				Type:        def.Type,
				Path:        path,
				ProjectPath: home,
				ProjectName: "(global)",
				SizeBytes:   info.Size(),
				Modified:    info.ModTime().Format(time.RFC3339),
			})
		}
	}

	seen := map[string]bool{}
	for _, pp := range projectPaths {
		if pp == "" || seen[pp] {
			continue
		}
		seen[pp] = true

		projectName := filepath.Base(pp)

		for _, def := range knownInstructionFiles {
			path := filepath.Join(pp, def.Path)
			info, err := os.Stat(path)
			found := err == nil && !info.IsDir()
			logScan(path, "instruction-file", found)
			if !found {
				continue
			}

			files = append(files, InstructionFile{
				Type:        def.Type,
				Path:        path,
				ProjectPath: pp,
				ProjectName: projectName,
				SizeBytes:   info.Size(),
				Modified:    info.ModTime().Format(time.RFC3339),
			})
		}
	}

	return files
}

var knownRuleDirs = []struct {
	Type string
	Dir  string
	Ext  string
}{
	{"cursor-rules", ".cursor/rules", ".mdc"},
	{"windsurf-rules", ".windsurf/rules", ".md"},
	{"roo-rules", ".roo/rules", ""},
	{"claude-agents", ".claude/agents", ""},
	{"claude-commands", ".claude/commands", ".md"},
	{"copilot-instructions", ".github/instructions", ".md"},
	{"copilot-skills", ".github/skills", ""},
	{"cline-rules", ".clinerules", ""},
	{"amazonq-rules", ".amazonq/rules", ".md"},
	{"augment-rules", ".augment/rules", ".md"},
	{"trae-rules", ".trae/rules", ".md"},
	{"tabnine-guidelines", ".tabnine/guidelines", ".md"},
	{"void-rules", ".void/rules", ".md"},
	{"kilo-rules", ".kilo/rules", ""},
	{"kilocode-rules", ".kilocode/rules", ""},
	{"devin-skills", ".devin/skills", ""},
	{"gemini-extensions", ".gemini/extensions", ""},
	{"opencode-commands", ".opencode/commands", ".md"},
	{"opencode-agents", ".opencode/agents", ".md"},
}

func scanRuleDirs(projectPaths []string) []InstructionFile {
	var files []InstructionFile

	seen := map[string]bool{}
	for _, pp := range projectPaths {
		if pp == "" || seen[pp] {
			continue
		}
		seen[pp] = true
		projectName := filepath.Base(pp)

		for _, def := range knownRuleDirs {
			dir := filepath.Join(pp, def.Dir)
			entries, err := os.ReadDir(dir)
			if err != nil {
				logScan(dir, "rules-dir", false)
				continue
			}
			logScan(dir, "rules-dir", true)
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if def.Ext != "" && !strings.HasSuffix(e.Name(), def.Ext) {
					continue
				}
				path := filepath.Join(dir, e.Name())
				info, err := os.Stat(path)
				if err != nil {
					continue
				}
				files = append(files, InstructionFile{
					Type:        def.Type,
					Path:        path,
					ProjectPath: pp,
					ProjectName: projectName,
					SizeBytes:   info.Size(),
					Modified:    info.ModTime().Format(time.RFC3339),
				})
			}
		}
	}

	return files
}
