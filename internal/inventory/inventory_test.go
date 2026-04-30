package inventory

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"vibecockpit/internal/provider"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func resetScanLog() { scanLog = nil }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// decodeClaudeProjectDir
// ---------------------------------------------------------------------------

func TestDecodeClaudeProjectDir(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"-home-user-project", "/home/user/project"},
		{"-home-nils-Documents-Workspace-claude-helper", "/home/nils/Documents/Workspace/claude/helper"},
		{"plain", "plain"},
		{"-", "/"},
		{"-a", "/a"},
		{"-a-b-c", "/a/b/c"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := decodeClaudeProjectDir(tc.input)
			if got != tc.want {
				t.Errorf("decodeClaudeProjectDir(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// dedup
// ---------------------------------------------------------------------------

func TestDedup(t *testing.T) {
	t.Run("removes exact duplicates", func(t *testing.T) {
		servers := []MCPServer{
			{Name: "a", Command: "cmd", URL: ""},
			{Name: "a", Command: "cmd", URL: ""},
			{Name: "b", Command: "cmd2", URL: ""},
		}
		got := dedup(servers)
		if len(got) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(got))
		}
	})

	t.Run("keeps different names", func(t *testing.T) {
		servers := []MCPServer{
			{Name: "a", Command: "cmd"},
			{Name: "b", Command: "cmd"},
		}
		got := dedup(servers)
		if len(got) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(got))
		}
	})

	t.Run("keeps same name different command", func(t *testing.T) {
		servers := []MCPServer{
			{Name: "a", Command: "cmd1"},
			{Name: "a", Command: "cmd2"},
		}
		got := dedup(servers)
		if len(got) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(got))
		}
	})

	t.Run("keeps same name different url", func(t *testing.T) {
		servers := []MCPServer{
			{Name: "a", URL: "http://one"},
			{Name: "a", URL: "http://two"},
		}
		got := dedup(servers)
		if len(got) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(got))
		}
	})

	t.Run("nil input", func(t *testing.T) {
		got := dedup(nil)
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		got := dedup([]MCPServer{})
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// scanClaudeMCP
// ---------------------------------------------------------------------------

func TestScanClaudeMCP(t *testing.T) {
	resetScanLog()

	t.Run("mcpServers map", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{
			"mcpServers": {
				"my-server": {
					"command": "npx",
					"args": ["-y", "my-server"],
					"url": ""
				},
				"remote-one": {
					"url": "https://example.com/mcp"
				}
			}
		}`)

		got := scanClaudeMCP(p, "test-source", "global")
		if len(got) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(got))
		}

		byName := map[string]MCPServer{}
		for _, s := range got {
			byName[s.Name] = s
		}

		s := byName["my-server"]
		if s.Command != "npx" {
			t.Errorf("command = %q, want npx", s.Command)
		}
		if len(s.Args) != 2 || s.Args[0] != "-y" || s.Args[1] != "my-server" {
			t.Errorf("args = %v, want [-y my-server]", s.Args)
		}
		if s.Source != "test-source" {
			t.Errorf("source = %q, want test-source", s.Source)
		}
		if s.Scope != "global" {
			t.Errorf("scope = %q, want global", s.Scope)
		}
		if s.SourcePath != p {
			t.Errorf("sourcePath = %q, want %q", s.SourcePath, p)
		}

		r := byName["remote-one"]
		if r.URL != "https://example.com/mcp" {
			t.Errorf("url = %q, want https://example.com/mcp", r.URL)
		}
	})

	t.Run("enabledMcpjsonServers", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{
			"enabledMcpjsonServers": ["remote-a", "remote-b"]
		}`)

		got := scanClaudeMCP(p, "src", "sc")
		if len(got) != 2 {
			t.Fatalf("expected 2, got %d", len(got))
		}
		for _, s := range got {
			if s.URL != "(remote)" {
				t.Errorf("expected URL (remote), got %q", s.URL)
			}
		}
	})

	t.Run("permissions mcp__name__tool", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{
			"permissions": {
				"allow": [
					"mcp__github__create_issue",
					"mcp__github__list_issues",
					"mcp__slack__send_message",
					"Bash(npm test)"
				]
			}
		}`)

		got := scanClaudeMCP(p, "src", "sc")
		if len(got) != 2 {
			t.Fatalf("expected 2 (github, slack), got %d: %+v", len(got), got)
		}

		names := map[string]bool{}
		for _, s := range got {
			names[s.Name] = true
			if s.URL != "(referenced)" {
				t.Errorf("expected URL (referenced), got %q", s.URL)
			}
			if s.Source != "src (permissions)" {
				t.Errorf("source = %q, want 'src (permissions)'", s.Source)
			}
		}
		if !names["github"] || !names["slack"] {
			t.Errorf("expected github and slack, got %v", names)
		}
	})

	t.Run("combined mcpServers + enabledMcpjsonServers + permissions", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{
			"mcpServers": {"local": {"command": "node"}},
			"enabledMcpjsonServers": ["remote"],
			"permissions": {"allow": ["mcp__perm-server__tool"]}
		}`)

		got := scanClaudeMCP(p, "s", "g")
		if len(got) != 3 {
			t.Fatalf("expected 3, got %d: %+v", len(got), got)
		}
	})

	t.Run("missing file returns nil", func(t *testing.T) {
		got := scanClaudeMCP("/nonexistent/path.json", "s", "g")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("malformed JSON returns nil", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "bad.json")
		writeFile(t, p, `{not valid json}`)

		got := scanClaudeMCP(p, "s", "g")
		if got != nil {
			t.Fatalf("expected nil for malformed JSON, got %v", got)
		}
	})

	t.Run("empty file returns nil", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "empty.json")
		writeFile(t, p, ``)

		got := scanClaudeMCP(p, "s", "g")
		if got != nil {
			t.Fatalf("expected nil for empty file, got %v", got)
		}
	})

	t.Run("empty mcpServers map", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{"mcpServers": {}}`)

		got := scanClaudeMCP(p, "s", "g")
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})

	t.Run("mcpServers with non-object value is skipped", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{"mcpServers": "not-a-map"}`)

		got := scanClaudeMCP(p, "s", "g")
		// mcpServers unmarshal to map fails, so no servers from that branch
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})

	t.Run("permissions with empty allow array", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{"permissions": {"allow": []}}`)

		got := scanClaudeMCP(p, "s", "g")
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// scanClaudePluginsMCP
// ---------------------------------------------------------------------------

func TestScanClaudePluginsMCP(t *testing.T) {
	t.Run("v2 format", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "installed_plugins.json")
		writeFile(t, p, `{
			"version": 2,
			"plugins": {
				"my-plugin@marketplace": [
					{"scope": "global", "installPath": "/usr/bin/my-plugin", "projectPath": ""},
					{"scope": "project", "installPath": "/usr/bin/my-plugin", "projectPath": "/home/user/proj"}
				],
				"other@npm": [
					{"scope": "global", "installPath": "/usr/bin/other", "projectPath": ""}
				]
			}
		}`)

		got := scanClaudePluginsMCP(p)
		if len(got) != 3 {
			t.Fatalf("expected 3 servers, got %d: %+v", len(got), got)
		}

		// Check name stripping of @marketplace
		for _, s := range got {
			if strings.Contains(s.Name, "@") {
				t.Errorf("name should not contain @: %q", s.Name)
			}
			if s.Source != "claude-plugin" {
				t.Errorf("source = %q, want claude-plugin", s.Source)
			}
		}

		// Check project-scoped entry
		projectScoped := false
		for _, s := range got {
			if s.Scope == "/home/user/proj" {
				projectScoped = true
			}
		}
		if !projectScoped {
			t.Error("expected a project-scoped entry with projectPath")
		}
	})

	t.Run("v1 format", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "installed_plugins.json")
		writeFile(t, p, `[
			{"name": "plugin-a", "package_name": "pkg-a"},
			{"name": "", "package_name": "pkg-b"},
			{"name": "plugin-c", "package_name": ""},
			{"name": "", "package_name": ""}
		]`)

		got := scanClaudePluginsMCP(p)
		if len(got) != 3 {
			t.Fatalf("expected 3 (skip empty name+package), got %d: %+v", len(got), got)
		}

		names := map[string]bool{}
		for _, s := range got {
			names[s.Name] = true
		}
		if !names["plugin-a"] || !names["pkg-b"] || !names["plugin-c"] {
			t.Errorf("unexpected names: %v", names)
		}
	})

	t.Run("v1 name fallback to package_name", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "installed_plugins.json")
		writeFile(t, p, `[{"name": "", "package_name": "fallback-name"}]`)

		got := scanClaudePluginsMCP(p)
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d", len(got))
		}
		if got[0].Name != "fallback-name" {
			t.Errorf("name = %q, want fallback-name", got[0].Name)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		got := scanClaudePluginsMCP("/nonexistent.json")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "bad.json")
		writeFile(t, p, `{{{bad}}}`)

		got := scanClaudePluginsMCP(p)
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("v2 with version 1 falls through to v1", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "plugins.json")
		// version=1 does not satisfy v2.Version >= 2, so falls to v1 parse (which fails on object)
		writeFile(t, p, `{"version": 1, "plugins": {"x@m": [{"scope":"global"}]}}`)

		got := scanClaudePluginsMCP(p)
		// v1 parse of an object also fails, so nil
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty v2 plugins map", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "plugins.json")
		writeFile(t, p, `{"version": 2, "plugins": {}}`)

		got := scanClaudePluginsMCP(p)
		// v2.Version >= 2 but len(v2.Plugins) == 0, falls through to v1
		// v1 parse of object fails
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// scanCursorMCP
// ---------------------------------------------------------------------------

func TestScanCursorMCP(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "mcp.json")
		writeFile(t, p, `{
			"mcpServers": {
				"server-a": {
					"command": "node",
					"args": ["index.js"],
					"url": ""
				},
				"server-b": {
					"url": "https://remote.example.com"
				}
			}
		}`)

		got := scanCursorMCP(p)
		if len(got) != 2 {
			t.Fatalf("expected 2, got %d", len(got))
		}

		byName := map[string]MCPServer{}
		for _, s := range got {
			byName[s.Name] = s
		}

		a := byName["server-a"]
		if a.Command != "node" {
			t.Errorf("command = %q, want node", a.Command)
		}
		if a.Source != "cursor" {
			t.Errorf("source = %q, want cursor", a.Source)
		}
		if len(a.Args) != 1 || a.Args[0] != "index.js" {
			t.Errorf("args = %v, want [index.js]", a.Args)
		}

		b := byName["server-b"]
		if b.URL != "https://remote.example.com" {
			t.Errorf("url = %q", b.URL)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		got := scanCursorMCP("/nonexistent.json")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "bad.json")
		writeFile(t, p, `not json`)

		got := scanCursorMCP(p)
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty mcpServers", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "mcp.json")
		writeFile(t, p, `{"mcpServers": {}}`)

		got := scanCursorMCP(p)
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// scanZedMCP
// ---------------------------------------------------------------------------

func TestScanZedMCP(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{
			"context_servers": {
				"my-ctx": {
					"command": "my-binary",
					"args": ["--verbose"]
				}
			}
		}`)

		got := scanZedMCP(p)
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d", len(got))
		}
		if got[0].Name != "my-ctx" {
			t.Errorf("name = %q", got[0].Name)
		}
		if got[0].Command != "my-binary" {
			t.Errorf("command = %q", got[0].Command)
		}
		if got[0].Source != "zed" {
			t.Errorf("source = %q", got[0].Source)
		}
		if len(got[0].Args) != 1 || got[0].Args[0] != "--verbose" {
			t.Errorf("args = %v", got[0].Args)
		}
	})

	t.Run("empty context_servers returns nil", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{"context_servers": {}}`)

		got := scanZedMCP(p)
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		got := scanZedMCP("/nonexistent.json")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("no context_servers key", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "settings.json")
		writeFile(t, p, `{"theme": "dark"}`)

		got := scanZedMCP(p)
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "bad.json")
		writeFile(t, p, `{bad}`)

		got := scanZedMCP(p)
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// scanInstructionFiles
// ---------------------------------------------------------------------------

func TestScanInstructionFiles(t *testing.T) {
	resetScanLog()

	t.Run("finds known files in project", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# Instructions")
		writeFile(t, filepath.Join(dir, ".cursorrules"), "rules here")
		writeFile(t, filepath.Join(dir, ".github", "copilot-instructions.md"), "copilot")

		got := scanInstructionFiles([]string{dir})

		// Filter to only entries from our temp dir
		var found []InstructionFile
		for _, f := range got {
			if f.ProjectPath == dir {
				found = append(found, f)
			}
		}

		if len(found) != 3 {
			t.Fatalf("expected 3 files, got %d: %+v", len(found), found)
		}

		types := map[string]bool{}
		for _, f := range found {
			types[f.Type] = true
			if f.ProjectName != filepath.Base(dir) {
				t.Errorf("projectName = %q, want %q", f.ProjectName, filepath.Base(dir))
			}
			if f.SizeBytes <= 0 {
				t.Errorf("sizeBytes should be > 0, got %d", f.SizeBytes)
			}
			if f.Modified == "" {
				t.Error("modified should not be empty")
			}
		}

		if !types["CLAUDE.md"] || !types[".cursorrules"] || !types["copilot-instructions.md"] {
			t.Errorf("missing expected types: %v", types)
		}
	})

	t.Run("empty project list", func(t *testing.T) {
		got := scanInstructionFiles(nil)
		// May contain global files from the real home dir; we just check no panic
		_ = got
	})

	t.Run("nonexistent project path", func(t *testing.T) {
		got := scanInstructionFiles([]string{"/nonexistent/path"})
		// Should find zero project files, only possibly global ones
		for _, f := range got {
			if f.ProjectPath == "/nonexistent/path" {
				t.Error("should not find files in nonexistent path")
			}
		}
	})

	t.Run("deduplicates project paths", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "CLAUDE.md"), "x")

		got := scanInstructionFiles([]string{dir, dir, dir})

		count := 0
		for _, f := range got {
			if f.ProjectPath == dir && f.Type == "CLAUDE.md" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected CLAUDE.md once, got %d", count)
		}
	})

	t.Run("skips directories with same name as instruction file", func(t *testing.T) {
		dir := t.TempDir()
		// Create CLAUDE.md as a directory, not a file
		os.MkdirAll(filepath.Join(dir, "CLAUDE.md"), 0o755)

		got := scanInstructionFiles([]string{dir})
		for _, f := range got {
			if f.ProjectPath == dir && f.Type == "CLAUDE.md" {
				t.Error("should not pick up CLAUDE.md directory")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// scanRuleDirs
// ---------------------------------------------------------------------------

func TestScanRuleDirs(t *testing.T) {
	resetScanLog()

	t.Run("finds .cursor/rules/*.mdc files", func(t *testing.T) {
		dir := t.TempDir()
		rulesDir := filepath.Join(dir, ".cursor", "rules")
		writeFile(t, filepath.Join(rulesDir, "rule1.mdc"), "rule 1")
		writeFile(t, filepath.Join(rulesDir, "rule2.mdc"), "rule 2")
		writeFile(t, filepath.Join(rulesDir, "ignore.txt"), "wrong ext")

		got := scanRuleDirs([]string{dir})
		var found []InstructionFile
		for _, f := range got {
			if f.ProjectPath == dir {
				found = append(found, f)
			}
		}

		if len(found) != 2 {
			t.Fatalf("expected 2 .mdc files, got %d: %+v", len(found), found)
		}
		for _, f := range found {
			if f.Type != "cursor-rules" {
				t.Errorf("type = %q, want cursor-rules", f.Type)
			}
			if !strings.HasSuffix(f.Path, ".mdc") {
				t.Errorf("path should end with .mdc: %s", f.Path)
			}
		}
	})

	t.Run("finds .windsurf/rules/*.md files", func(t *testing.T) {
		dir := t.TempDir()
		rulesDir := filepath.Join(dir, ".windsurf", "rules")
		writeFile(t, filepath.Join(rulesDir, "ws-rule.md"), "windsurf")

		got := scanRuleDirs([]string{dir})
		var found []InstructionFile
		for _, f := range got {
			if f.ProjectPath == dir && f.Type == "windsurf-rules" {
				found = append(found, f)
			}
		}

		if len(found) != 1 {
			t.Fatalf("expected 1, got %d", len(found))
		}
	})

	t.Run("rules dir with no ext filter picks up all files", func(t *testing.T) {
		dir := t.TempDir()
		// .roo/rules has Ext="" so it picks up anything
		rulesDir := filepath.Join(dir, ".roo", "rules")
		writeFile(t, filepath.Join(rulesDir, "custom.yaml"), "yaml")
		writeFile(t, filepath.Join(rulesDir, "other.txt"), "txt")

		got := scanRuleDirs([]string{dir})
		var found []InstructionFile
		for _, f := range got {
			if f.ProjectPath == dir && f.Type == "roo-rules" {
				found = append(found, f)
			}
		}

		if len(found) != 2 {
			t.Fatalf("expected 2 files with no ext filter, got %d", len(found))
		}
	})

	t.Run("skips subdirectories within rule dirs", func(t *testing.T) {
		dir := t.TempDir()
		rulesDir := filepath.Join(dir, ".cursor", "rules")
		writeFile(t, filepath.Join(rulesDir, "good.mdc"), "ok")
		os.MkdirAll(filepath.Join(rulesDir, "subdir"), 0o755)

		got := scanRuleDirs([]string{dir})
		var found []InstructionFile
		for _, f := range got {
			if f.ProjectPath == dir {
				found = append(found, f)
			}
		}

		if len(found) != 1 {
			t.Fatalf("expected 1, got %d", len(found))
		}
	})

	t.Run("missing rule dir is not an error", func(t *testing.T) {
		dir := t.TempDir()
		got := scanRuleDirs([]string{dir})
		// No rule dirs exist — should return empty without error
		for _, f := range got {
			if f.ProjectPath == dir {
				t.Error("should find nothing in empty project")
			}
		}
	})

	t.Run("deduplicates project paths", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".cursor", "rules", "r.mdc"), "x")

		got := scanRuleDirs([]string{dir, dir})
		count := 0
		for _, f := range got {
			if f.ProjectPath == dir {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected 1, got %d (dedup failed)", count)
		}
	})
}

// ---------------------------------------------------------------------------
// aggregateModels
// ---------------------------------------------------------------------------

func TestAggregateModels(t *testing.T) {
	t.Run("groups by model and sorts by count", func(t *testing.T) {
		now := time.Now()
		sessions := []provider.Session{
			{Model: "claude-3-opus", Provider: "anthropic", Modified: now.Add(-2 * time.Hour)},
			{Model: "claude-3-opus", Provider: "anthropic", Modified: now.Add(-1 * time.Hour)},
			{Model: "claude-3-opus", Provider: "anthropic", Modified: now},
			{Model: "gpt-4", Provider: "openai", Modified: now},
		}

		got := aggregateModels(sessions)
		if len(got) != 2 {
			t.Fatalf("expected 2 models, got %d", len(got))
		}

		// First should be most used (claude-3-opus with 3 sessions)
		if got[0].Model != "claude-3-opus" {
			t.Errorf("first model = %q, want claude-3-opus", got[0].Model)
		}
		if got[0].SessionCount != 3 {
			t.Errorf("sessionCount = %d, want 3", got[0].SessionCount)
		}
		if got[0].Provider != "anthropic" {
			t.Errorf("provider = %q, want anthropic", got[0].Provider)
		}

		if got[1].Model != "gpt-4" {
			t.Errorf("second model = %q, want gpt-4", got[1].Model)
		}
		if got[1].SessionCount != 1 {
			t.Errorf("sessionCount = %d, want 1", got[1].SessionCount)
		}
	})

	t.Run("skips sessions with empty model", func(t *testing.T) {
		sessions := []provider.Session{
			{Model: "", Provider: "x", Modified: time.Now()},
			{Model: "claude", Provider: "y", Modified: time.Now()},
		}

		got := aggregateModels(sessions)
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d", len(got))
		}
		if got[0].Model != "claude" {
			t.Errorf("model = %q", got[0].Model)
		}
	})

	t.Run("provider tracks most recent session", func(t *testing.T) {
		now := time.Now()
		sessions := []provider.Session{
			{Model: "m1", Provider: "old-provider", Modified: now.Add(-1 * time.Hour)},
			{Model: "m1", Provider: "new-provider", Modified: now},
		}

		got := aggregateModels(sessions)
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d", len(got))
		}
		if got[0].Provider != "new-provider" {
			t.Errorf("provider = %q, want new-provider", got[0].Provider)
		}
	})

	t.Run("empty sessions", func(t *testing.T) {
		got := aggregateModels(nil)
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})

	t.Run("lastUsed is RFC3339 formatted", func(t *testing.T) {
		now := time.Now()
		sessions := []provider.Session{
			{Model: "m", Provider: "p", Modified: now},
		}

		got := aggregateModels(sessions)
		if _, err := time.Parse(time.RFC3339, got[0].LastUsed); err != nil {
			t.Errorf("lastUsed %q is not valid RFC3339: %v", got[0].LastUsed, err)
		}
	})
}

// ---------------------------------------------------------------------------
// scanCommandDir
// ---------------------------------------------------------------------------

func TestScanCommandDir(t *testing.T) {
	t.Run("reads .md files", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "deploy.md"), "deploy instructions")
		writeFile(t, filepath.Join(dir, "test.md"), "test instructions")
		writeFile(t, filepath.Join(dir, "ignore.txt"), "not a command")

		got := scanCommandDir(dir, "test-source")
		if len(got) != 2 {
			t.Fatalf("expected 2, got %d", len(got))
		}

		names := map[string]bool{}
		for _, s := range got {
			names[s.Name] = true
			if s.Type != "command" {
				t.Errorf("type = %q, want command", s.Type)
			}
			if s.Source != "test-source" {
				t.Errorf("source = %q, want test-source", s.Source)
			}
		}
		if !names["deploy"] || !names["test"] {
			t.Errorf("unexpected names: %v", names)
		}
	})

	t.Run("skips directories", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "subdir.md"), 0o755)
		writeFile(t, filepath.Join(dir, "real.md"), "content")

		got := scanCommandDir(dir, "s")
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d", len(got))
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		got := scanCommandDir("/nonexistent/dir", "s")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		got := scanCommandDir(dir, "s")
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})

	t.Run("strips .md extension from name", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "my-command.md"), "x")

		got := scanCommandDir(dir, "s")
		if got[0].Name != "my-command" {
			t.Errorf("name = %q, want my-command", got[0].Name)
		}
	})
}

// ---------------------------------------------------------------------------
// scanClaudePlugins (skills.go)
// ---------------------------------------------------------------------------

func TestScanClaudePlugins(t *testing.T) {
	t.Run("v2 format", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "installed_plugins.json")
		writeFile(t, p, `{
			"version": 2,
			"plugins": {
				"my-plugin@marketplace": [
					{"scope": "global", "projectPath": ""}
				],
				"another@npm": [
					{"scope": "project", "projectPath": "/home/user/proj"}
				]
			}
		}`)

		got := scanClaudePlugins(p)
		if len(got) != 2 {
			t.Fatalf("expected 2, got %d", len(got))
		}

		names := map[string]bool{}
		for _, s := range got {
			names[s.Name] = true
			if s.Type != "plugin" {
				t.Errorf("type = %q, want plugin", s.Type)
			}
			if s.Source != "claude-plugin" {
				t.Errorf("source = %q", s.Source)
			}
		}
		if !names["my-plugin"] || !names["another"] {
			t.Errorf("names = %v", names)
		}
	})

	t.Run("v1 format", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "installed_plugins.json")
		writeFile(t, p, `[
			{"name": "skill-a", "package_name": "pkg-a"},
			{"name": "", "package_name": "pkg-b"}
		]`)

		got := scanClaudePlugins(p)
		if len(got) != 2 {
			t.Fatalf("expected 2, got %d", len(got))
		}
		if got[0].Name != "skill-a" {
			t.Errorf("name = %q", got[0].Name)
		}
		if got[1].Name != "pkg-b" {
			t.Errorf("fallback name = %q, want pkg-b", got[1].Name)
		}
	})

	t.Run("v1 skips entries with no name or package", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "installed_plugins.json")
		writeFile(t, p, `[{"name": "", "package_name": ""}]`)

		got := scanClaudePlugins(p)
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})

	t.Run("missing file", func(t *testing.T) {
		got := scanClaudePlugins("/no/such/file.json")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "bad.json")
		writeFile(t, p, `{{{`)

		got := scanClaudePlugins(p)
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// parseMemoryFrontmatter
// ---------------------------------------------------------------------------

func TestParseMemoryFrontmatter(t *testing.T) {
	t.Run("full frontmatter", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "test.md")
		writeFile(t, p, `---
name: My Memory
description: A test memory file
type: instruction
---
# Content here
Some body text.
`)

		name, desc, mtype := parseMemoryFrontmatter(p)
		if name != "My Memory" {
			t.Errorf("name = %q, want 'My Memory'", name)
		}
		if desc != "A test memory file" {
			t.Errorf("description = %q", desc)
		}
		if mtype != "instruction" {
			t.Errorf("type = %q, want instruction", mtype)
		}
	})

	t.Run("partial frontmatter", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "test.md")
		writeFile(t, p, `---
name: Just Name
---
body
`)

		name, desc, mtype := parseMemoryFrontmatter(p)
		if name != "Just Name" {
			t.Errorf("name = %q", name)
		}
		if desc != "" {
			t.Errorf("description should be empty, got %q", desc)
		}
		if mtype != "" {
			t.Errorf("type should be empty, got %q", mtype)
		}
	})

	t.Run("no frontmatter", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "test.md")
		writeFile(t, p, `# Just a markdown file
No frontmatter here.
`)

		name, desc, mtype := parseMemoryFrontmatter(p)
		if name != "" || desc != "" || mtype != "" {
			t.Errorf("expected empty values, got name=%q desc=%q type=%q", name, desc, mtype)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "empty.md")
		writeFile(t, p, "")

		name, desc, mtype := parseMemoryFrontmatter(p)
		if name != "" || desc != "" || mtype != "" {
			t.Errorf("expected empty values for empty file")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		name, desc, mtype := parseMemoryFrontmatter("/no/such/file.md")
		if name != "" || desc != "" || mtype != "" {
			t.Errorf("expected empty values for missing file")
		}
	})

	t.Run("frontmatter with extra whitespace", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "test.md")
		writeFile(t, p, `---
name:   Spaced Name
description:  Spaced Desc
---
`)

		name, desc, _ := parseMemoryFrontmatter(p)
		if name != "Spaced Name" {
			t.Errorf("name = %q, want 'Spaced Name'", name)
		}
		if desc != "Spaced Desc" {
			t.Errorf("description = %q, want 'Spaced Desc'", desc)
		}
	})

	t.Run("frontmatter with unknown keys ignored", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "test.md")
		writeFile(t, p, `---
name: Known
author: Unknown Key
---
`)

		name, _, _ := parseMemoryFrontmatter(p)
		if name != "Known" {
			t.Errorf("name = %q", name)
		}
	})
}

// ---------------------------------------------------------------------------
// scanMemories (integration-like with temp dirs)
// ---------------------------------------------------------------------------

func TestScanMemories(t *testing.T) {
	resetScanLog()

	// scanMemories reads from ~/.claude/projects/*/memory/, which is the
	// real home dir.  We can only unit-test parseMemoryFrontmatter and
	// decodeClaudeProjectDir directly; scanMemories itself depends on
	// os.UserHomeDir().  We still verify it doesn't panic with empty input.

	t.Run("empty project paths does not panic", func(t *testing.T) {
		resetScanLog()
		got := scanMemories(nil)
		// May or may not return items depending on real home dir state.
		_ = got
	})

	t.Run("skips MEMORY.md", func(t *testing.T) {
		// This tests the filtering logic conceptually:
		// The function skips any file named MEMORY.md and non-.md files.
		// We verify this via parseMemoryFrontmatter on MEMORY.md just as
		// a sanity check — the real skip is in scanMemories.
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "MEMORY.md"), "skip me")
		writeFile(t, filepath.Join(dir, "other.txt"), "also skip")
		writeFile(t, filepath.Join(dir, "valid.md"), "---\nname: Valid\n---\ncontent")

		// Can't easily test scanMemories with a mock home dir, but we verify
		// parseMemoryFrontmatter works for the valid file.
		name, _, _ := parseMemoryFrontmatter(filepath.Join(dir, "valid.md"))
		if name != "Valid" {
			t.Errorf("name = %q, want Valid", name)
		}
	})
}

// ---------------------------------------------------------------------------
// collectProjectPaths
// ---------------------------------------------------------------------------

func TestCollectProjectPaths(t *testing.T) {
	t.Run("deduplicates session paths", func(t *testing.T) {
		sessions := []provider.Session{
			{ProjectPath: "/a"},
			{ProjectPath: "/a"},
			{ProjectPath: "/b"},
		}

		got := collectProjectPaths(sessions, "")
		if len(got) != 2 {
			t.Fatalf("expected 2, got %d: %v", len(got), got)
		}
	})

	t.Run("skips empty project paths", func(t *testing.T) {
		sessions := []provider.Session{
			{ProjectPath: ""},
			{ProjectPath: "/valid"},
		}

		got := collectProjectPaths(sessions, "")
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d: %v", len(got), got)
		}
		if got[0] != "/valid" {
			t.Errorf("got %q", got[0])
		}
	})

	t.Run("adds workspace subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "proj-a"), 0o755)
		os.MkdirAll(filepath.Join(dir, "proj-b"), 0o755)
		writeFile(t, filepath.Join(dir, "file.txt"), "not a dir")

		got := collectProjectPaths(nil, dir)
		if len(got) != 2 {
			t.Fatalf("expected 2 dirs, got %d: %v", len(got), got)
		}

		sort.Strings(got)
		if got[0] != filepath.Join(dir, "proj-a") || got[1] != filepath.Join(dir, "proj-b") {
			t.Errorf("unexpected paths: %v", got)
		}
	})

	t.Run("deduplicates across sessions and workspace", func(t *testing.T) {
		dir := t.TempDir()
		subDir := filepath.Join(dir, "my-proj")
		os.MkdirAll(subDir, 0o755)

		sessions := []provider.Session{
			{ProjectPath: subDir},
		}

		got := collectProjectPaths(sessions, dir)
		count := 0
		for _, p := range got {
			if p == subDir {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected subDir once, found %d times in %v", count, got)
		}
	})

	t.Run("empty sessions and empty workspace", func(t *testing.T) {
		got := collectProjectPaths(nil, "")
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})

	t.Run("nonexistent workspace dir", func(t *testing.T) {
		got := collectProjectPaths(nil, "/nonexistent/workspace")
		if len(got) != 0 {
			t.Fatalf("expected 0, got %d", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// logScan
// ---------------------------------------------------------------------------

func TestLogScan(t *testing.T) {
	resetScanLog()

	logScan("/test/path", "test-type", true)
	logScan("/other/path", "other-type", false)

	if len(scanLog) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(scanLog))
	}

	if scanLog[0].Path != "/test/path" || scanLog[0].Type != "test-type" || !scanLog[0].Found {
		t.Errorf("entry 0 = %+v", scanLog[0])
	}
	if scanLog[1].Path != "/other/path" || scanLog[1].Type != "other-type" || scanLog[1].Found {
		t.Errorf("entry 1 = %+v", scanLog[1])
	}
}

// ---------------------------------------------------------------------------
// scanSkills (integration-like)
// ---------------------------------------------------------------------------

func TestScanSkills(t *testing.T) {
	resetScanLog()

	t.Run("finds per-project commands and SKILLS", func(t *testing.T) {
		dir := t.TempDir()
		cmdDir := filepath.Join(dir, ".claude", "commands")
		writeFile(t, filepath.Join(cmdDir, "deploy.md"), "deploy instructions")
		writeFile(t, filepath.Join(cmdDir, "test.md"), "test instructions")

		skillsDir := filepath.Join(dir, "SKILLS")
		writeFile(t, filepath.Join(skillsDir, "analyze.md"), "analyze skill")

		got := scanSkills([]string{dir})

		// Filter to our project (global scan may add things from real home)
		var found []Skill
		for _, s := range got {
			if strings.Contains(s.Path, dir) {
				found = append(found, s)
			}
		}

		if len(found) != 3 {
			t.Fatalf("expected 3, got %d: %+v", len(found), found)
		}

		// Check that the SKILLS/analyze.md entry has type "skill"
		var skillEntry *Skill
		for i, s := range found {
			if strings.Contains(s.Path, filepath.Join("SKILLS", "analyze.md")) {
				skillEntry = &found[i]
			}
		}
		if skillEntry == nil {
			t.Fatal("did not find SKILLS/analyze.md entry")
		}
		if skillEntry.Type != "skill" {
			t.Errorf("SKILLS entry should have type=skill, got %q", skillEntry.Type)
		}
		if skillEntry.Name != "analyze" {
			t.Errorf("SKILLS entry name = %q, want analyze", skillEntry.Name)
		}
	})

	t.Run("empty project list does not panic", func(t *testing.T) {
		resetScanLog()
		got := scanSkills(nil)
		_ = got // may have global results
	})
}

// ── Sensitive files ──

func TestScanSensitiveFiles(t *testing.T) {
	t.Run("finds .env and .env variants at root", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".env"), "SECRET=x")
		writeFile(t, filepath.Join(dir, ".env.local"), "LOCAL=y")
		writeFile(t, filepath.Join(dir, ".env.custom"), "CUSTOM=z")

		got := scanSensitiveFiles([]string{dir})
		names := map[string]bool{}
		for _, f := range got {
			names[f.Name] = true
			if f.ProjectName == "" {
				t.Error("missing project name")
			}
			if f.Risk == "" {
				t.Error("missing risk for", f.Name)
			}
		}
		for _, want := range []string{".env", ".env.local", ".env.custom"} {
			if !names[want] {
				t.Errorf("expected to find %s", want)
			}
		}
	})

	t.Run("finds sensitive files in subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "config")
		os.MkdirAll(sub, 0755)
		writeFile(t, filepath.Join(sub, ".env"), "DB_PASS=x")
		writeFile(t, filepath.Join(sub, "credentials.json"), "{}")

		keys := filepath.Join(dir, "deploy")
		os.MkdirAll(keys, 0755)
		writeFile(t, filepath.Join(keys, "server.pem"), "---")

		got := scanSensitiveFiles([]string{dir})
		names := map[string]bool{}
		for _, f := range got {
			names[f.Name] = true
		}
		for _, want := range []string{
			filepath.Join("config", ".env"),
			filepath.Join("config", "credentials.json"),
			filepath.Join("deploy", "server.pem"),
		} {
			if !names[want] {
				t.Errorf("expected to find %s, got %v", want, names)
			}
		}
	})

	t.Run("skips node_modules and .git", func(t *testing.T) {
		dir := t.TempDir()
		nm := filepath.Join(dir, "node_modules", "some-pkg")
		os.MkdirAll(nm, 0755)
		writeFile(t, filepath.Join(nm, ".env"), "SHOULD_SKIP")

		gitDir := filepath.Join(dir, ".git", "hooks")
		os.MkdirAll(gitDir, 0755)
		writeFile(t, filepath.Join(gitDir, "secret.key"), "SHOULD_SKIP")

		got := scanSensitiveFiles([]string{dir})
		if len(got) != 0 {
			names := []string{}
			for _, f := range got {
				names = append(names, f.Name)
			}
			t.Errorf("expected 0 findings in skipped dirs, got %v", names)
		}
	})

	t.Run("respects max depth of 2", func(t *testing.T) {
		dir := t.TempDir()
		// depth 0 (root) - should find
		writeFile(t, filepath.Join(dir, ".env"), "ROOT")
		// depth 1 - should find
		ok := filepath.Join(dir, "config")
		os.MkdirAll(ok, 0755)
		writeFile(t, filepath.Join(ok, ".env.local"), "STILL_OK")
		// depth 3 - beyond limit, should be skipped
		deep := filepath.Join(dir, "a", "b", "c")
		os.MkdirAll(deep, 0755)
		writeFile(t, filepath.Join(deep, ".env"), "TOO_DEEP")

		got := scanSensitiveFiles([]string{dir})
		names := map[string]bool{}
		for _, f := range got {
			names[f.Name] = true
		}
		if !names[".env"] {
			t.Error("should find root .env")
		}
		if !names[filepath.Join("config", ".env.local")] {
			t.Error("should find depth-1 .env.local")
		}
		deepName := filepath.Join("a", "b", "c", ".env")
		if names[deepName] {
			t.Error("should NOT find depth-3 .env")
		}
	})

	t.Run("finds credential and key files", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "credentials.json"), "{}")
		writeFile(t, filepath.Join(dir, "server.pem"), "---")
		writeFile(t, filepath.Join(dir, "deploy.key"), "---")

		got := scanSensitiveFiles([]string{dir})
		names := map[string]bool{}
		for _, f := range got {
			names[f.Name] = true
		}
		for _, want := range []string{"credentials.json", "server.pem", "deploy.key"} {
			if !names[want] {
				t.Errorf("expected to find %s", want)
			}
		}
	})

	t.Run("finds .docker/config.json in subdirectory", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".docker"), 0755)
		writeFile(t, filepath.Join(dir, ".docker", "config.json"), "{}")

		got := scanSensitiveFiles([]string{dir})
		found := false
		for _, f := range got {
			if f.Name == filepath.Join(".docker", "config.json") {
				found = true
			}
		}
		if !found {
			t.Error("expected to find .docker/config.json")
		}
	})

	t.Run("ignores directories named .env", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".env"), 0755)

		got := scanSensitiveFiles([]string{dir})
		for _, f := range got {
			if f.Name == ".env" {
				t.Error(".env directory should be ignored")
			}
		}
	})

	t.Run("empty project list returns nil", func(t *testing.T) {
		got := scanSensitiveFiles(nil)
		if got != nil {
			t.Errorf("expected nil, got %d entries", len(got))
		}
	})

	t.Run("deduplicates project paths", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".env"), "X=1")

		got := scanSensitiveFiles([]string{dir, dir, dir})
		count := 0
		for _, f := range got {
			if f.Name == ".env" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected 1 .env entry, got %d", count)
		}
	})

	t.Run("excludes .env.example and .env.sample templates", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".env"), "SECRET=x")
		writeFile(t, filepath.Join(dir, ".env.example"), "SECRET=")
		writeFile(t, filepath.Join(dir, ".env.sample"), "SECRET=")
		writeFile(t, filepath.Join(dir, ".env.template"), "SECRET=")
		writeFile(t, filepath.Join(dir, ".env.dist"), "SECRET=")

		got := scanSensitiveFiles([]string{dir})
		for _, f := range got {
			if f.Name != ".env" {
				t.Errorf("template file %s should be excluded", f.Name)
			}
		}
		if len(got) != 1 {
			t.Errorf("expected 1 finding (.env only), got %d", len(got))
		}
	})

	t.Run("excludes templates in subdirectories too", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "services")
		os.MkdirAll(sub, 0755)
		writeFile(t, filepath.Join(sub, ".env.example"), "TEMPLATE")
		writeFile(t, filepath.Join(sub, ".env"), "REAL_SECRET")

		got := scanSensitiveFiles([]string{dir})
		if len(got) != 1 {
			t.Errorf("expected 1 finding, got %d", len(got))
		}
		if len(got) > 0 && got[0].Name != filepath.Join("services", ".env") {
			t.Errorf("expected services/.env, got %s", got[0].Name)
		}
	})

	t.Run("clean project has no findings", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "main.go"), "package main")
		writeFile(t, filepath.Join(dir, "README.md"), "# Hello")
		os.MkdirAll(filepath.Join(dir, "src"), 0755)
		writeFile(t, filepath.Join(dir, "src", "app.go"), "package src")

		got := scanSensitiveFiles([]string{dir})
		if len(got) != 0 {
			t.Errorf("expected 0 findings, got %d", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// parseExtension (extensions.go)
// ---------------------------------------------------------------------------

func TestParseExtension(t *testing.T) {
	t.Run("known AI extension by ID", func(t *testing.T) {
		dir := t.TempDir()
		pkg := filepath.Join(dir, "github.copilot-1.0.0", "package.json")
		writeFile(t, pkg, `{
			"name": "copilot",
			"displayName": "GitHub Copilot",
			"version": "1.0.0",
			"publisher": "GitHub",
			"description": "Your AI pair programmer",
			"categories": ["Programming Languages"]
		}`)

		got := parseExtension(pkg, "vscode", "VS Code")
		if got == nil {
			t.Fatal("expected extension, got nil")
		}
		if got.ID != "github.copilot" {
			t.Errorf("id = %q, want github.copilot", got.ID)
		}
		if got.Name != "GitHub Copilot" {
			t.Errorf("name = %q", got.Name)
		}
		if got.Version != "1.0.0" {
			t.Errorf("version = %q", got.Version)
		}
		if got.Publisher != "GitHub" {
			t.Errorf("publisher = %q", got.Publisher)
		}
		if got.IDE != "VS Code" {
			t.Errorf("ide = %q", got.IDE)
		}
		if got.IDEID != "vscode" {
			t.Errorf("ideId = %q", got.IDEID)
		}
	})

	t.Run("AI extension by category", func(t *testing.T) {
		dir := t.TempDir()
		pkg := filepath.Join(dir, "unknown.ai-tool-1.0.0", "package.json")
		writeFile(t, pkg, `{
			"name": "ai-tool",
			"displayName": "Some AI Tool",
			"version": "2.0.0",
			"publisher": "Unknown",
			"categories": ["AI", "Chat"]
		}`)

		got := parseExtension(pkg, "cursor", "Cursor")
		if got == nil {
			t.Fatal("expected extension, got nil")
		}
		if got.ID != "unknown.ai-tool" {
			t.Errorf("id = %q", got.ID)
		}
		if got.IDE != "Cursor" {
			t.Errorf("ide = %q", got.IDE)
		}
	})

	t.Run("Machine Learning alone is not detected", func(t *testing.T) {
		dir := t.TempDir()
		pkg := filepath.Join(dir, "ext", "package.json")
		writeFile(t, pkg, `{
			"name": "ml-helper",
			"displayName": "ML Helper",
			"version": "1.0.0",
			"publisher": "test",
			"categories": ["Machine Learning"]
		}`)

		got := parseExtension(pkg, "vscode", "VS Code")
		if got != nil {
			t.Error("Machine Learning alone should not be detected as AI extension")
		}
	})

	t.Run("non-AI extension skipped", func(t *testing.T) {
		dir := t.TempDir()
		pkg := filepath.Join(dir, "esbenp.prettier-3.0.0", "package.json")
		writeFile(t, pkg, `{
			"name": "prettier-vscode",
			"displayName": "Prettier",
			"version": "3.0.0",
			"publisher": "esbenp",
			"categories": ["Formatters"]
		}`)

		got := parseExtension(pkg, "vscode", "VS Code")
		if got != nil {
			t.Errorf("expected nil for non-AI extension, got %+v", got)
		}
	})

	t.Run("missing package.json returns nil", func(t *testing.T) {
		got := parseExtension("/nonexistent/package.json", "vscode", "VS Code")
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("malformed JSON returns nil", func(t *testing.T) {
		dir := t.TempDir()
		pkg := filepath.Join(dir, "bad", "package.json")
		writeFile(t, pkg, `{not json}`)

		got := parseExtension(pkg, "vscode", "VS Code")
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("falls back to name when displayName empty", func(t *testing.T) {
		dir := t.TempDir()
		pkg := filepath.Join(dir, "ext", "package.json")
		writeFile(t, pkg, `{
			"name": "copilot",
			"displayName": "",
			"version": "1.0.0",
			"publisher": "GitHub",
			"categories": ["AI"]
		}`)

		got := parseExtension(pkg, "vscode", "VS Code")
		if got == nil {
			t.Fatal("expected extension")
		}
		if got.Name != "copilot" {
			t.Errorf("name = %q, want copilot (fallback)", got.Name)
		}
	})

	t.Run("publisher is lowercased in ID", func(t *testing.T) {
		dir := t.TempDir()
		pkg := filepath.Join(dir, "ext", "package.json")
		writeFile(t, pkg, `{
			"name": "Copilot",
			"displayName": "GitHub Copilot",
			"version": "1.0.0",
			"publisher": "GitHub",
			"categories": ["AI"]
		}`)

		got := parseExtension(pkg, "vscode", "VS Code")
		if got == nil {
			t.Fatal("expected extension")
		}
		if got.ID != "github.copilot" {
			t.Errorf("id = %q, want github.copilot (lowercase)", got.ID)
		}
	})
}

func TestIsAIExtension(t *testing.T) {
	tests := []struct {
		id         string
		categories []string
		want       bool
	}{
		{"github.copilot", nil, true},
		{"continue.continue", nil, true},
		{"saoudrizwan.claude-dev", nil, true},
		{"esbenp.prettier", nil, false},
		{"random.thing", []string{"AI"}, true},
		{"random.thing", []string{"ai"}, true},
		{"random.thing", []string{"Machine Learning"}, false},
		{"random.thing", []string{"AI", "Machine Learning"}, true},
		{"random.thing", []string{"Formatters", "Linters"}, false},
		{"random.thing", nil, false},
		{"github.vscode-pull-request-github", []string{"AI"}, false},
		{"ms-python.python", []string{"Machine Learning"}, false},
		{"ms-toolsai.jupyter", []string{"AI", "Machine Learning"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := isAIExtension(tc.id, tc.categories)
			if got != tc.want {
				t.Errorf("isAIExtension(%q, %v) = %v, want %v", tc.id, tc.categories, got, tc.want)
			}
		})
	}
}
