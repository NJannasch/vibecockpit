package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var aiExtensionIDs = map[string]bool{
	"github.copilot":                      true,
	"github.copilot-chat":                 true,
	"continue.continue":                   true,
	"saoudrizwan.claude-dev":              true, // Cline
	"rooveterinaryinc.roo-cline":          true, // Roo Code
	"sourcegraph.cody-ai":                 true,
	"amazonwebservices.aws-toolkit-vscode": true, // Amazon Q
	"amazonwebservices.amazon-q-vscode":   true,
	"tabnine.tabnine-vscode":              true,
	"codium.codium":                       true, // Codium / Qodo
	"supermaven.supermaven":               true,
	"anthropics.claude-code":              true,
	"anthropic.claude-code":               true,
	"codeium.codeium":                     true, // Codeium / Windsurf
	"codeium.windsurf":                    true,
	"augment.augment-vscode":              true,
	"cursor.cursor":                       true,
	"aider.aider":                         true,
}

var notAIExtensions = map[string]bool{
	"github.vscode-pull-request-github": true,
	"github.github-vscode-theme":        true,
	"ms-python.python":                  true,
	"ms-python.vscode-pylance":          true,
	"ms-python.debugpy":                 true,
	"ms-python.isort":                   true,
	"ms-toolsai.jupyter":                true,
	"ms-toolsai.jupyter-renderers":      true,
	"ms-toolsai.jupyter-keymap":         true,
	"ms-toolsai.vscode-jupyter-cell-tags": true,
	"ms-toolsai.vscode-jupyter-slideshow": true,
}

type ideDef struct {
	ID     string
	Name   string
	RelDir string // relative to home
}

func ideExtDirs() []ideDef {
	defs := []ideDef{
		{ID: "vscode", Name: "VS Code", RelDir: ".vscode/extensions"},
		{ID: "vscode-insiders", Name: "VS Code Insiders", RelDir: ".vscode-insiders/extensions"},
		{ID: "cursor", Name: "Cursor", RelDir: ".cursor/extensions"},
		{ID: "windsurf", Name: "Windsurf", RelDir: ".windsurf/extensions"},
		{ID: "trae", Name: "Trae", RelDir: ".trae/extensions"},
	}
	if runtime.GOOS == "darwin" {
		defs = append(defs, ideDef{ID: "zed", Name: "Zed", RelDir: "Library/Application Support/Zed/extensions"})
	} else {
		defs = append(defs, ideDef{ID: "zed", Name: "Zed", RelDir: ".local/share/zed/extensions"})
	}
	return defs
}

type extPkgJSON struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Version     string   `json:"version"`
	Publisher   string   `json:"publisher"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
}

func scanIDEExtensions() []IDEExtension {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}

	var results []IDEExtension
	for _, ide := range ideExtDirs() {
		dir := filepath.Join(home, ide.RelDir)
		logScan(tildefy(home, dir), "ide-extensions", false)

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		scanLog[len(scanLog)-1].Found = true

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			pkgPath := filepath.Join(dir, e.Name(), "package.json")
			ext := parseExtension(pkgPath, ide.ID, ide.Name)
			if ext != nil {
				results = append(results, *ext)
			}
		}
	}

	return results
}

func parseExtension(pkgPath, ideID, ideName string) *IDEExtension {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil
	}

	var pkg extPkgJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	qualifiedID := strings.ToLower(pkg.Publisher) + "." + strings.ToLower(pkg.Name)

	if !isAIExtension(qualifiedID, pkg.Categories) {
		return nil
	}

	displayName := pkg.DisplayName
	if displayName == "" {
		displayName = pkg.Name
	}

	var installed string
	if info, err := os.Stat(pkgPath); err == nil {
		installed = info.ModTime().Format(time.RFC3339)
	}

	return &IDEExtension{
		ID:          qualifiedID,
		Name:        displayName,
		Version:     pkg.Version,
		Publisher:   pkg.Publisher,
		IDE:         ideName,
		IDEID:       ideID,
		Description: pkg.Description,
		Installed:   installed,
	}
}

func isAIExtension(qualifiedID string, categories []string) bool {
	if notAIExtensions[qualifiedID] {
		return false
	}
	if aiExtensionIDs[qualifiedID] {
		return true
	}
	hasAI := false
	for _, cat := range categories {
		if strings.EqualFold(cat, "AI") {
			hasAI = true
		}
	}
	return hasAI
}

func tildefy(home, path string) string {
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
