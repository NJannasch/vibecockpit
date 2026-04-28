package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Terminal != "default" {
		t.Errorf("Terminal = %q, want %q", cfg.Terminal, "default")
	}
	if cfg.Theme != "light" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "light")
	}
	if cfg.SortBy != "modified" {
		t.Errorf("SortBy = %q, want %q", cfg.SortBy, "modified")
	}
	home, _ := os.UserHomeDir()
	wantDir := filepath.Join(home, "Documents", "Workspace")
	if cfg.NewProjectDir != wantDir {
		t.Errorf("NewProjectDir = %q, want %q", cfg.NewProjectDir, wantDir)
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := Load()
	if cfg.Terminal != "default" {
		t.Errorf("expected default terminal, got %q", cfg.Terminal)
	}
	if cfg.Theme != "light" {
		t.Errorf("expected default theme, got %q", cfg.Theme)
	}
}

func TestLoadValidYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "vibecockpit")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte("terminal: kitty\nnew_project_dir: /tmp/projects\ntheme: light\nsort_by: name\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.Terminal != "kitty" {
		t.Errorf("Terminal = %q, want kitty", cfg.Terminal)
	}
	if cfg.NewProjectDir != "/tmp/projects" {
		t.Errorf("NewProjectDir = %q, want /tmp/projects", cfg.NewProjectDir)
	}
	if cfg.Theme != "light" {
		t.Errorf("Theme = %q, want light", cfg.Theme)
	}
	if cfg.SortBy != "name" {
		t.Errorf("SortBy = %q, want name", cfg.SortBy)
	}
}

func TestLoadPartialYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "vibecockpit")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte("terminal: alacritty\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.Terminal != "alacritty" {
		t.Errorf("Terminal = %q, want alacritty", cfg.Terminal)
	}
	// Fields not in YAML keep their defaults from Default()
	if cfg.Theme != "light" {
		t.Errorf("Theme = %q, want light (default)", cfg.Theme)
	}
	if cfg.SortBy != "modified" {
		t.Errorf("SortBy = %q, want modified (default)", cfg.SortBy)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "vibecockpit")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte(":::bad yaml{{{")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	// Should still return a valid config (defaults)
	if cfg.Terminal != "default" {
		t.Errorf("Terminal = %q, want default after bad YAML", cfg.Terminal)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := &Config{
		Terminal:      "wezterm",
		NewProjectDir: "/home/test/projects",
		Theme:         "light",
		SortBy:        "name",
		DisabledProviders: []string{"copilot", "antigravity"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded := Load()
	if loaded.Terminal != cfg.Terminal {
		t.Errorf("Terminal = %q, want %q", loaded.Terminal, cfg.Terminal)
	}
	if loaded.NewProjectDir != cfg.NewProjectDir {
		t.Errorf("NewProjectDir = %q, want %q", loaded.NewProjectDir, cfg.NewProjectDir)
	}
	if loaded.Theme != cfg.Theme {
		t.Errorf("Theme = %q, want %q", loaded.Theme, cfg.Theme)
	}
	if loaded.SortBy != cfg.SortBy {
		t.Errorf("SortBy = %q, want %q", loaded.SortBy, cfg.SortBy)
	}
	if len(loaded.DisabledProviders) != 2 || loaded.DisabledProviders[0] != "copilot" {
		t.Errorf("DisabledProviders = %v, want [copilot antigravity]", loaded.DisabledProviders)
	}
}

func TestPathWithXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	want := "/custom/config/vibecockpit/config.yaml"
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestPathWithoutXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "vibecockpit", "config.yaml")
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}
