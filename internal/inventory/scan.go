package inventory

import (
	"os"
	"path/filepath"
	"time"

	"vibecockpit/internal/provider"
)

var scanLog []ScanEntry

func logScan(path, scanType string, found bool) {
	scanLog = append(scanLog, ScanEntry{Path: path, Type: scanType, Found: found})
}

func Scan(sessions []provider.Session, workspaceDir string) *Inventory {
	start := time.Now()
	scanLog = nil
	projectPaths := collectProjectPaths(sessions, workspaceDir)

	inv := &Inventory{
		Tools:            scanTools(),
		Models:           aggregateModels(sessions),
		MCPServers:       scanMCPServers(projectPaths),
		InstructionFiles: append(scanInstructionFiles(projectPaths), scanRuleDirs(projectPaths)...),
		Skills:           scanSkills(projectPaths),
		Memories:         scanMemories(projectPaths),
		SensitiveFiles:   scanSensitiveFiles(projectPaths),
		IDEExtensions:    scanIDEExtensions(),
		ScanLog:          scanLog,
		ProjectPaths:     projectPaths,
		ScanDurationMs:   time.Since(start).Milliseconds(),
		ScannedAt:        time.Now(),
	}
	return inv
}

func collectProjectPaths(sessions []provider.Session, workspaceDir string) []string {
	seen := map[string]bool{}
	var paths []string

	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	for _, s := range sessions {
		add(s.ProjectPath)
	}

	if workspaceDir != "" {
		entries, err := os.ReadDir(workspaceDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					add(filepath.Join(workspaceDir, e.Name()))
				}
			}
		}
	}

	return paths
}
