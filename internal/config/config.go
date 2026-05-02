package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version       string   `yaml:"version,omitempty"`
	Terminal      string   `yaml:"terminal"`
	CustomTermCmd string   `yaml:"custom_terminal_cmd,omitempty"`
	NewProjectDir string   `yaml:"new_project_dir"`
	Theme         string   `yaml:"theme,omitempty"`
	SortBy        string   `yaml:"sort_by,omitempty"`
	GroupBy       string            `yaml:"group_by,omitempty"`
	DisabledProviders []string                `yaml:"disabled_providers,omitempty"`
	ProviderPaths map[string]string          `yaml:"provider_paths,omitempty"`
	PluginConfigs   map[string]map[string]any `yaml:"plugins,omitempty"`
	RemoteSources   []map[string]any          `yaml:"remote_sources,omitempty"`
	ExtraPath       []string                  `yaml:"extra_path,omitempty"`
	AgentPrompt      string                    `yaml:"agent_prompt,omitempty"`
	ToolConfigFiles  map[string]string         `yaml:"tool_config_files,omitempty"`
	EnableMCP        bool                      `yaml:"enable_mcp,omitempty"`
	EnableScanner   bool                      `yaml:"enable_scanner,omitempty"`
	ScanSkipRules   []string                  `yaml:"scan_skip_rules,omitempty"`
	ScanExtraHints  []string                  `yaml:"scan_extra_hints,omitempty"`
}

var Models = []string{
	"claude-opus-4-7",
	"claude-opus-4-6[1m]",
	"claude-opus-4-6",
	"claude-sonnet-4-6",
	"claude-haiku-4-5-20251001",
}

var allTerminals = []string{
	"kitty",
	"alacritty",
	"wezterm",
	"ghostty",
	"hyper",
	"gnome-terminal",
	"konsole",
	"xterm",
	"iterm2",
	"terminal.app",
}

var (
	availableOnce      sync.Once
	availableTerminals []string
)

func AvailableTerminals() []string {
	availableOnce.Do(func() {
		availableTerminals = []string{"default"}
		for _, t := range allTerminals {
			if _, err := exec.LookPath(t); err == nil {
				availableTerminals = append(availableTerminals, t)
			}
		}
		availableTerminals = append(availableTerminals, "custom")
	})
	return availableTerminals
}

func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Terminal:      "default",
		NewProjectDir: filepath.Join(home, "Documents", "Workspace"),
		Theme:         "light",
		SortBy:        "modified",
	}
}

func Load() *Config {
	cfg := Default()
	data, err := os.ReadFile(Path())
	if err != nil {
		return cfg
	}
	_ = yaml.Unmarshal(data, cfg)
	return cfg
}

func (c *Config) Save() error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

func Path() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "vibecockpit", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vibecockpit", "config.yaml")
}
