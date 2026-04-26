package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vibecockpit/internal/config"
)

func TestResolveBinary_WithProviderPaths(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "my-claude")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		ProviderPaths: map[string]string{
			"claude": fakeBin,
		},
	}

	got := resolveBinary(cfg, "claude", "claude")
	if got != fakeBin {
		t.Errorf("expected %q, got %q", fakeBin, got)
	}
}

func TestResolveBinary_FallbackToLookPath(t *testing.T) {
	cfg := &config.Config{
		ProviderPaths: map[string]string{},
	}

	// "ls" should be found on any system via LookPath
	got := resolveBinary(cfg, "test-provider", "ls")
	if got == "" {
		t.Error("expected to find 'ls' via LookPath, got empty")
	}
}

func TestResolveBinary_NotFound(t *testing.T) {
	cfg := &config.Config{
		ProviderPaths: map[string]string{},
	}

	got := resolveBinary(cfg, "test-provider", "this-binary-definitely-does-not-exist-xyz")
	if got != "" {
		t.Errorf("expected empty for nonexistent binary, got %q", got)
	}
}

func TestResolveBinary_ProviderPathNonexistentFallsThrough(t *testing.T) {
	cfg := &config.Config{
		ProviderPaths: map[string]string{
			"test-provider": "/nonexistent/path/to/binary",
		},
	}

	// The configured path does not exist, so it should fall through to LookPath.
	// "ls" should be found via LookPath.
	got := resolveBinary(cfg, "test-provider", "ls")
	if got == "" {
		t.Error("expected fallback to LookPath for 'ls', got empty")
	}
	// It should NOT be the nonexistent path
	if got == "/nonexistent/path/to/binary" {
		t.Error("should not return the nonexistent configured path")
	}
}

func TestExpandHome_TildePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	got := expandHome("~/foo/bar")
	want := home + "/foo/bar"
	if got != want {
		t.Errorf("expandHome(~/foo/bar) = %q, want %q", got, want)
	}
}

func TestExpandHome_AbsolutePath(t *testing.T) {
	got := expandHome("/usr/local/bin/claude")
	if got != "/usr/local/bin/claude" {
		t.Errorf("expandHome should not change absolute path, got %q", got)
	}
}

func TestExpandHome_RelativePath(t *testing.T) {
	got := expandHome("relative/path")
	if strings.HasPrefix(got, "/home") || strings.HasPrefix(got, "/Users") {
		t.Errorf("expandHome should not expand non-tilde relative path, got %q", got)
	}
	if got != "relative/path" {
		t.Errorf("expected 'relative/path', got %q", got)
	}
}
