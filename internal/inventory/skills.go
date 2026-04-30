package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func scanSkills(projectPaths []string) []Skill {
	var skills []Skill
	home, _ := os.UserHomeDir()

	// Claude Code global commands
	globalCmds := filepath.Join(home, ".claude", "commands")
	found := scanCommandDir(globalCmds, "claude-global")
	logScan(globalCmds, "commands-dir", len(found) > 0)
	skills = append(skills, found...)

	// Claude Code installed plugins
	pluginPath := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	pf := scanClaudePlugins(pluginPath)
	logScan(pluginPath, "plugins", len(pf) > 0)
	skills = append(skills, pf...)

	// Per-project commands and SKILLS directories
	seen := map[string]bool{}
	for _, pp := range projectPaths {
		if pp == "" || seen[pp] {
			continue
		}
		seen[pp] = true
		projName := filepath.Base(pp)

		cmdDir := filepath.Join(pp, ".claude", "commands")
		f := scanCommandDir(cmdDir, "project:"+projName)
		logScan(cmdDir, "commands-dir", len(f) > 0)
		skills = append(skills, f...)

		skillsDir := filepath.Join(pp, "SKILLS")
		sf := scanCommandDir(skillsDir, "project:"+projName)
		logScan(skillsDir, "skills-dir", len(sf) > 0)
		for i := range sf {
			sf[i].Type = "skill"
		}
		skills = append(skills, sf...)
	}

	return skills
}

func scanCommandDir(dir, source string) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var skills []Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		skills = append(skills, Skill{
			Name:   name,
			Type:   "command",
			Source: source,
			Path:   filepath.Join(dir, e.Name()),
		})
	}
	return skills
}

func scanClaudePlugins(path string) []Skill {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// v2 format: {"version": 2, "plugins": {"name@marketplace": [...]}}
	var v2 struct {
		Version int                        `json:"version"`
		Plugins map[string][]struct {
			Scope       string `json:"scope"`
			ProjectPath string `json:"projectPath"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &v2) == nil && v2.Version >= 2 && len(v2.Plugins) > 0 {
		var skills []Skill
		for fullName := range v2.Plugins {
			name := fullName
			if i := strings.Index(name, "@"); i > 0 {
				name = name[:i]
			}
			skills = append(skills, Skill{
				Name:   name,
				Type:   "plugin",
				Source: "claude-plugin",
				Path:   path,
			})
		}
		return skills
	}

	// v1 fallback
	var v1 []struct {
		Name    string `json:"name"`
		Package string `json:"package_name"`
	}
	if json.Unmarshal(data, &v1) == nil {
		var skills []Skill
		for _, p := range v1 {
			n := p.Name
			if n == "" {
				n = p.Package
			}
			if n == "" {
				continue
			}
			skills = append(skills, Skill{
				Name:   n,
				Type:   "plugin",
				Source: "claude-plugin",
				Path:   path,
			})
		}
		return skills
	}

	return nil
}
