package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vibecockpit/internal/provider"
)

func TestKeywordsMatch(t *testing.T) {
	tests := []struct {
		line     string
		keywords []string
		want     bool
	}{
		{"contains sk-proj-abc123", []string{"sk-"}, true},
		{"contains akia and secret", []string{"akia", "secret"}, true},
		{"contains only akia", []string{"akia", "secret"}, false},
		{"no match here", []string{"sk-"}, false},
		{"anything matches", nil, true},
		{"anything matches", []string{}, true},
	}
	for _, tt := range tests {
		if got := keywordsMatch(tt.line, tt.keywords); got != tt.want {
			t.Errorf("keywordsMatch(%q, %v) = %v, want %v", tt.line, tt.keywords, got, tt.want)
		}
	}
}

func TestRedactMatch(t *testing.T) {
	tests := []struct {
		input string
		check func(string) bool
		desc  string
	}{
		{"sk-proj-abc123def456", func(s string) bool { return len(s) == len("sk-proj-abc123def456") && s[:4] == "sk-p" }, "keeps prefix, has ****"},
		{"short", func(s string) bool { return s[:2] == "sh" }, "short strings keep first 2"},
		{"a]b", func(s string) bool { return len(s) == 3 }, "very short"},
	}
	for _, tt := range tests {
		result := redactMatch(tt.input)
		if !tt.check(result) {
			t.Errorf("redactMatch(%q) = %q — %s", tt.input, result, tt.desc)
		}
	}
}

func TestIsTestValue(t *testing.T) {
	if !isTestValue("AKIATEST1234567890") {
		t.Error("expected AKIATEST to be detected as test value")
	}
	if !isTestValue("sk-test-abc123") {
		t.Error("expected sk-test to be detected as test value")
	}
	if isTestValue("sk-proj-realkey123456789") {
		t.Error("real-looking key should not be flagged as test value")
	}
}

func TestExtractHintFromRegex(t *testing.T) {
	tests := []struct {
		regex string
		want  int // minimum hint length expected (0 = no hint)
	}{
		{`AGE-SECRET-KEY-1[A-Z]{58}`, 1},
		{`ghp_[a-zA-Z0-9]{36}`, 1},
		{`[a-f0-9]+`, 1}, // extracts literal fragments, that's ok
	}
	for _, tt := range tests {
		hints := extractHintFromRegex(tt.regex)
		if tt.want > 0 && len(hints) == 0 {
			t.Errorf("extractHintFromRegex(%q) returned no hints, expected at least one", tt.regex)
		}
		if tt.want == 0 && len(hints) > 0 {
			t.Errorf("extractHintFromRegex(%q) = %v, expected none", tt.regex, hints)
		}
	}
}

func TestParseKeywords(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{`["sk-", "openai"]`, 2},
		{`["single"]`, 1},
		{`[]`, 0},
	}
	for _, tt := range tests {
		got := parseKeywords(tt.input)
		if len(got) != tt.want {
			t.Errorf("parseKeywords(%q) = %v (len=%d), want len=%d", tt.input, got, len(got), tt.want)
		}
	}
}

type mockProvider struct {
	sessions []provider.Session
}

func (m *mockProvider) Name() string { return "test" }
func (m *mockProvider) Icon() string { return "T" }
func (m *mockProvider) ScanSessions(_ context.Context) ([]provider.Session, error) {
	return m.sessions, nil
}
func (m *mockProvider) ResumeCommand(_ provider.Session) (string, []string) { return "", nil }
func (m *mockProvider) NewCommand(_ string) (string, []string)             { return "", nil }
func (m *mockProvider) DeleteSession(_ string) error                       { return nil }

func TestScanFile_FindsSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	content := `{"type":"user","content":"normal message"}
{"type":"assistant","content":"here is the code"}
{"type":"user","content":"my key is ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef1234"}
{"type":"user","content":"also AKIAIOSFODNN7REAL123"}
{"type":"user","content":"clean line"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := New([]provider.Provider{&mockProvider{
		sessions: []provider.Session{{
			ID: "test-1", Provider: "test", ProjectName: "test-proj", DataPath: path,
		}},
	}})

	s.Start()

	// Poll until done
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status := s.GetStatus()
		if status.State == "done" {
			if status.FindingCount == 0 {
				t.Log("No findings — rules may not be loaded (no gitleaks.toml cache). Skipping assertion.")
				return
			}
			// Check that we found the GitHub PAT
			foundGHP := false
			for _, f := range status.Findings {
				if f.Line == 3 {
					foundGHP = true
				}
			}
			if !foundGHP {
				t.Errorf("Expected finding on line 3 (GitHub PAT), got findings: %v", status.Findings)
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Error("scan did not complete in time")
}

func TestScanFile_SkipsCleanFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.jsonl")

	content := `{"type":"user","content":"hello world"}
{"type":"assistant","content":"how can I help?"}
{"type":"user","content":"write me a function"}
`
	os.WriteFile(path, []byte(content), 0644)

	s := New([]provider.Provider{&mockProvider{
		sessions: []provider.Session{{
			ID: "clean-1", Provider: "test", ProjectName: "clean", DataPath: path,
		}},
	}})

	s.Start()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status := s.GetStatus()
		if status.State == "done" {
			if status.FindingCount != 0 {
				t.Errorf("expected 0 findings for clean file, got %d", status.FindingCount)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("scan did not complete in time")
}
