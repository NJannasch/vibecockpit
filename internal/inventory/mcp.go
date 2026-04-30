package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func scanMCPServers(projectPaths []string) []MCPServer {
	var servers []MCPServer

	home, _ := os.UserHomeDir()

	// Claude Code global settings
	p := filepath.Join(home, ".claude", "settings.json")
	found := scanClaudeMCP(p, "claude-global", "global")
	logScan(p, "mcp-config", len(found) > 0)
	servers = append(servers, found...)

	// Claude Code global MCP config (~/.claude.json — distinct from settings.json)
	cj := filepath.Join(home, ".claude.json")
	cjf := scanClaudeMCP(cj, "claude-global", "global")
	logScan(cj, "mcp-config", len(cjf) > 0)
	servers = append(servers, cjf...)

	// Claude Code installed plugins (these are MCP servers)
	pluginsPath := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	pluginMCPs := scanClaudePluginsMCP(pluginsPath)
	logScan(pluginsPath, "mcp-plugins", len(pluginMCPs) > 0)
	servers = append(servers, pluginMCPs...)

	// Claude Code per-project settings
	projectsDir := filepath.Join(home, ".claude", "projects")
	if entries, err := os.ReadDir(projectsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			projDir := filepath.Join(projectsDir, e.Name())
			for _, name := range []string{"settings.json", "settings.local.json"} {
				sp := filepath.Join(projDir, name)
				scope := decodeClaudeProjectDir(e.Name())
				f := scanClaudeMCP(sp, "claude-project", scope)
				logScan(sp, "mcp-config", len(f) > 0)
				servers = append(servers, f...)
			}
		}
	}

	// Claude Desktop
	if runtime.GOOS == "darwin" {
		dp := filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
		f := scanClaudeMCP(dp, "claude-desktop", "global")
		logScan(dp, "mcp-config", len(f) > 0)
		servers = append(servers, f...)
	} else {
		dp := filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
		f := scanClaudeMCP(dp, "claude-desktop", "global")
		logScan(dp, "mcp-config", len(f) > 0)
		servers = append(servers, f...)
	}

	// Cursor MCP config
	cp := filepath.Join(home, ".cursor", "mcp.json")
	cf := scanCursorMCP(cp)
	logScan(cp, "mcp-config", len(cf) > 0)
	servers = append(servers, cf...)

	// Windsurf MCP config
	wsMCP := filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	wsf := scanClaudeMCP(wsMCP, "windsurf", "global")
	logScan(wsMCP, "mcp-config", len(wsf) > 0)
	servers = append(servers, wsf...)

	// Cline MCP config
	clinePath := filepath.Join(home, ".config", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json")
	clf := scanClaudeMCP(clinePath, "cline", "global")
	logScan(clinePath, "mcp-config", len(clf) > 0)
	servers = append(servers, clf...)

	// Roo Code MCP config
	rooPath := filepath.Join(home, ".config", "Code", "User", "globalStorage", "rooveterinaryinc.roo-cline", "settings", "mcp_settings.json")
	rof := scanClaudeMCP(rooPath, "roo-code", "global")
	logScan(rooPath, "mcp-config", len(rof) > 0)
	servers = append(servers, rof...)

	// Zed context servers
	zedPath := filepath.Join(home, ".config", "zed", "settings.json")
	zf := scanZedMCP(zedPath)
	logScan(zedPath, "mcp-config", len(zf) > 0)
	servers = append(servers, zf...)

	// Junie MCP config
	juniePath := filepath.Join(home, ".junie", "mcp", "mcp.json")
	jf := scanClaudeMCP(juniePath, "junie", "global")
	logScan(juniePath, "mcp-config", len(jf) > 0)
	servers = append(servers, jf...)

	// Copilot CLI MCP config
	copilotMCP := filepath.Join(home, ".copilot", "mcp-config.json")
	copf := scanClaudeMCP(copilotMCP, "copilot", "global")
	logScan(copilotMCP, "mcp-config", len(copf) > 0)
	servers = append(servers, copf...)

	// Gemini CLI MCP config (inside settings.json)
	geminiMCP := filepath.Join(home, ".gemini", "settings.json")
	gemf := scanClaudeMCP(geminiMCP, "gemini", "global")
	logScan(geminiMCP, "mcp-config", len(gemf) > 0)
	servers = append(servers, gemf...)

	// Continue.dev MCP config
	continueMCP := filepath.Join(home, ".continue", "config.json")
	contf := scanClaudeMCP(continueMCP, "continue", "global")
	logScan(continueMCP, "mcp-config", len(contf) > 0)
	servers = append(servers, contf...)

	// Amazon Q Developer MCP config
	amazonqMCP := filepath.Join(home, ".aws", "amazonq", "mcp.json")
	aqf := scanClaudeMCP(amazonqMCP, "amazonq", "global")
	logScan(amazonqMCP, "mcp-config", len(aqf) > 0)
	servers = append(servers, aqf...)

	// Tabnine MCP config
	tabnineMCP := filepath.Join(home, ".tabnine", "mcp_servers.json")
	tnf := scanClaudeMCP(tabnineMCP, "tabnine", "global")
	logScan(tabnineMCP, "mcp-config", len(tnf) > 0)
	servers = append(servers, tnf...)

	// Amp (Sourcegraph) MCP config
	ampMCP := filepath.Join(home, ".config", "amp", "settings.json")
	ampf := scanClaudeMCP(ampMCP, "amp", "global")
	logScan(ampMCP, "mcp-config", len(ampf) > 0)
	servers = append(servers, ampf...)

	// Devin MCP config
	devinMCP := filepath.Join(home, ".config", "devin", "config.json")
	devf := scanClaudeMCP(devinMCP, "devin", "global")
	logScan(devinMCP, "mcp-config", len(devf) > 0)
	servers = append(servers, devf...)

	// Kilo Code MCP config
	kiloMCP := filepath.Join(home, ".config", "kilo", "kilo.jsonc")
	kilof := scanClaudeMCP(kiloMCP, "kilo", "global")
	logScan(kiloMCP, "mcp-config", len(kilof) > 0)
	servers = append(servers, kilof...)

	// Qwen Code MCP config
	qwenMCP := filepath.Join(home, ".qwen", "settings.json")
	qwf := scanClaudeMCP(qwenMCP, "qwen", "global")
	logScan(qwenMCP, "mcp-config", len(qwf) > 0)
	servers = append(servers, qwf...)

	// Per-project MCP configs
	seen := map[string]bool{}
	for _, pp := range projectPaths {
		if pp == "" || seen[pp] {
			continue
		}
		seen[pp] = true

		for _, name := range []string{
			".mcp.json",
			".claude/settings.json", ".claude/settings.local.json",
			".cursor/mcp.json",
			".roo/mcp.json",
			".junie/mcp/mcp.json",
			".vscode/mcp.json",
			".trae/mcp.json",
			".amazonq/mcp.json",
			".tabnine/mcp_servers.json",
			".amp/settings.json",
			".gemini/settings.json",
			".qwen/settings.json",
		} {
			fp := filepath.Join(pp, name)
			f := scanClaudeMCP(fp, "project", pp)
			logScan(fp, "mcp-config", len(f) > 0)
			servers = append(servers, f...)
		}
	}

	return dedup(servers)
}

func scanClaudeMCP(path, source, scope string) []MCPServer {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}

	var servers []MCPServer

	// Standard mcpServers map (local stdio servers)
	if raw, ok := doc["mcpServers"]; ok {
		var mcpMap map[string]json.RawMessage
		if json.Unmarshal(raw, &mcpMap) == nil {
			for name, cfg := range mcpMap {
				s := MCPServer{
					Name:       name,
					Source:     source,
					SourcePath: path,
					Scope:      scope,
				}
				var entry struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
					URL     string   `json:"url"`
				}
				if json.Unmarshal(cfg, &entry) == nil {
					s.Command = entry.Command
					s.Args = entry.Args
					s.URL = entry.URL
				}
				servers = append(servers, s)
			}
		}
	}

	// enabledMcpjsonServers (remote/OAuth MCP servers)
	if raw, ok := doc["enabledMcpjsonServers"]; ok {
		var names []string
		if json.Unmarshal(raw, &names) == nil {
			for _, name := range names {
				servers = append(servers, MCPServer{
					Name:       name,
					Source:     source,
					SourcePath: path,
					Scope:      scope,
					URL:        "(remote)",
				})
			}
		}
	}

	// Extract MCP server names from permissions (mcp__servername__tool)
	if raw, ok := doc["permissions"]; ok {
		var perms struct {
			Allow []string `json:"allow"`
		}
		if json.Unmarshal(raw, &perms) == nil {
			mcpNames := map[string]bool{}
			for _, p := range perms.Allow {
				if strings.HasPrefix(p, "mcp__") {
					parts := strings.SplitN(p, "__", 3)
					if len(parts) >= 2 {
						mcpNames[parts[1]] = true
					}
				}
			}
			for name := range mcpNames {
				servers = append(servers, MCPServer{
					Name:       name,
					Source:     source + " (permissions)",
					SourcePath: path,
					Scope:      scope,
					URL:        "(referenced)",
				})
			}
		}
	}

	return servers
}

func scanClaudePluginsMCP(path string) []MCPServer {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// v2 format: {"version": 2, "plugins": {"name@marketplace": [...]}}
	var v2 struct {
		Version int                                `json:"version"`
		Plugins map[string][]struct {
			Scope       string `json:"scope"`
			InstallPath string `json:"installPath"`
			ProjectPath string `json:"projectPath"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &v2) == nil && v2.Version >= 2 && len(v2.Plugins) > 0 {
		var servers []MCPServer
		for fullName, installs := range v2.Plugins {
			name := fullName
			if i := strings.Index(name, "@"); i > 0 {
				name = name[:i]
			}
			for _, inst := range installs {
				scope := "global"
				if inst.ProjectPath != "" {
					scope = inst.ProjectPath
				}
				servers = append(servers, MCPServer{
					Name:       name,
					Source:     "claude-plugin",
					SourcePath: path,
					Scope:      scope,
					Command:    inst.InstallPath,
				})
			}
		}
		return servers
	}

	// v1 fallback: array format
	var v1 []struct {
		Name    string `json:"name"`
		Package string `json:"package_name"`
	}
	if json.Unmarshal(data, &v1) == nil {
		var servers []MCPServer
		for _, p := range v1 {
			n := p.Name
			if n == "" {
				n = p.Package
			}
			if n == "" {
				continue
			}
			servers = append(servers, MCPServer{
				Name:       n,
				Source:     "claude-plugin",
				SourcePath: path,
				Scope:      "global",
			})
		}
		return servers
	}

	return nil
}

func scanCursorMCP(path string) []MCPServer {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var doc struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			URL     string   `json:"url"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}

	var servers []MCPServer
	for name, cfg := range doc.MCPServers {
		servers = append(servers, MCPServer{
			Name:       name,
			Command:    cfg.Command,
			Args:       cfg.Args,
			URL:        cfg.URL,
			Source:     "cursor",
			SourcePath: path,
			Scope:      "global",
		})
	}
	return servers
}

func scanZedMCP(path string) []MCPServer {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var doc struct {
		ContextServers map[string]json.RawMessage `json:"context_servers"`
	}
	if json.Unmarshal(data, &doc) != nil || len(doc.ContextServers) == 0 {
		return nil
	}

	var servers []MCPServer
	for name, cfg := range doc.ContextServers {
		s := MCPServer{
			Name:       name,
			Source:     "zed",
			SourcePath: path,
			Scope:      "global",
		}
		var entry struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if json.Unmarshal(cfg, &entry) == nil {
			s.Command = entry.Command
			s.Args = entry.Args
		}
		servers = append(servers, s)
	}
	return servers
}

func decodeClaudeProjectDir(encoded string) string {
	if strings.HasPrefix(encoded, "-") {
		return "/" + strings.ReplaceAll(encoded[1:], "-", "/")
	}
	return encoded
}

func dedup(servers []MCPServer) []MCPServer {
	type key struct{ name, command, url string }
	seen := map[key]bool{}
	var out []MCPServer
	for _, s := range servers {
		k := key{s.Name, s.Command, s.URL}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}
