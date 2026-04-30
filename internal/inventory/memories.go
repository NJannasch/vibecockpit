package inventory

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func scanMemories(projectPaths []string) []MemoryFile {
	var memories []MemoryFile

	home, _ := os.UserHomeDir()
	projectsDir := filepath.Join(home, ".claude", "projects")

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		logScan(projectsDir, "memory-dir", false)
		return nil
	}

	knownProjects := map[string]string{}
	for _, pp := range projectPaths {
		if pp != "" {
			knownProjects[pp] = filepath.Base(pp)
		}
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		memDir := filepath.Join(projectsDir, e.Name(), "memory")
		files, err := os.ReadDir(memDir)
		if err != nil {
			logScan(memDir, "memory-dir", false)
			continue
		}
		logScan(memDir, "memory-dir", true)

		projectPath := decodeClaudeProjectDir(e.Name())
		projectName := knownProjects[projectPath]
		if projectName == "" {
			projectName = filepath.Base(projectPath)
		}

		for _, f := range files {
			if f.IsDir() || f.Name() == "MEMORY.md" {
				continue
			}
			if !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			fp := filepath.Join(memDir, f.Name())
			info, err := os.Stat(fp)
			if err != nil {
				continue
			}

			m := MemoryFile{
				Name:        strings.TrimSuffix(f.Name(), ".md"),
				Path:        fp,
				ProjectPath: projectPath,
				ProjectName: projectName,
				SizeBytes:   info.Size(),
				Modified:    info.ModTime().Format(time.RFC3339),
			}

			name, desc, mtype := parseMemoryFrontmatter(fp)
			if name != "" {
				m.Name = name
			}
			m.Description = desc
			m.MemoryType = mtype

			memories = append(memories, m)
		}
	}

	return memories
}

func parseMemoryFrontmatter(path string) (name, description, memType string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break
		}
		if !inFrontmatter {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			switch k {
			case "name":
				name = v
			case "description":
				description = v
			case "type":
				memType = v
			}
		}
	}
	return
}
