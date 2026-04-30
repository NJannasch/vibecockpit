package inventory

import "time"

type ToolInfo struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Installed  bool     `json:"installed"`
	Version    string   `json:"version,omitempty"`
	BinaryPath string   `json:"binaryPath,omitempty"`
	ConfigDir  string   `json:"configDir,omitempty"`
	DataDir    string   `json:"dataDir,omitempty"`
}

type ModelUsage struct {
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	SessionCount int    `json:"sessionCount"`
	LastUsed     string `json:"lastUsed"`
}

type MCPServer struct {
	Name       string   `json:"name"`
	Command    string   `json:"command,omitempty"`
	Args       []string `json:"args,omitempty"`
	URL        string   `json:"url,omitempty"`
	Source     string   `json:"source"`
	SourcePath string   `json:"sourcePath"`
	Scope      string   `json:"scope"`
}

type InstructionFile struct {
	Type        string `json:"type"`
	Path        string `json:"path"`
	ProjectPath string `json:"projectPath"`
	ProjectName string `json:"projectName"`
	SizeBytes   int64  `json:"sizeBytes"`
	Modified    string `json:"modified"`
}

type Skill struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type MemoryFile struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MemoryType  string `json:"memoryType,omitempty"`
	Path        string `json:"path"`
	ProjectPath string `json:"projectPath"`
	ProjectName string `json:"projectName"`
	SizeBytes   int64  `json:"sizeBytes"`
	Modified    string `json:"modified,omitempty"`
}

type SensitiveFile struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	ProjectName string `json:"projectName"`
	Risk        string `json:"risk"`
}

type IDEExtension struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Publisher   string `json:"publisher"`
	IDE         string `json:"ide"`
	IDEID       string `json:"ideId"`
	Description string `json:"description,omitempty"`
	Installed   string `json:"installed,omitempty"`
}

type ScanEntry struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Found  bool   `json:"found"`
}

type Inventory struct {
	Tools            []ToolInfo        `json:"tools"`
	Models           []ModelUsage      `json:"models"`
	MCPServers       []MCPServer       `json:"mcpServers"`
	InstructionFiles []InstructionFile `json:"instructionFiles"`
	Skills           []Skill           `json:"skills"`
	Memories         []MemoryFile      `json:"memories"`
	SensitiveFiles   []SensitiveFile   `json:"sensitiveFiles"`
	IDEExtensions    []IDEExtension    `json:"ideExtensions"`
	ScanLog          []ScanEntry       `json:"scanLog"`
	ProjectPaths     []string          `json:"projectPaths"`
	ScanDurationMs   int64             `json:"scanDurationMs"`
	ScannedAt        time.Time         `json:"scannedAt"`
}
