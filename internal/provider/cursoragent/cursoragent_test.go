package cursoragent

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"vibecockpit/internal/provider"
)

func createTestDB(t *testing.T, dir string, meta sessionMeta, blobs []map[string]any) string {
	t.Helper()
	dbPath := filepath.Join(dir, "store.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Exec("CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT)")
	db.Exec("CREATE TABLE blobs (id TEXT PRIMARY KEY, data BLOB)")

	metaJSON, _ := json.Marshal(meta)
	hexVal := hex.EncodeToString(metaJSON)
	db.Exec("INSERT INTO meta (key, value) VALUES ('0', ?)", hexVal)

	for i, blob := range blobs {
		data, _ := json.Marshal(blob)
		db.Exec("INSERT INTO blobs (id, data) VALUES (?, ?)", string(rune('a'+i)), data)
	}

	return dbPath
}

func TestScanSession_Basic(t *testing.T) {
	dir := t.TempDir()
	wsHash := "workspace-abc"
	sessDir := filepath.Join(dir, wsHash, "sess-001")
	os.MkdirAll(sessDir, 0755)

	meta := sessionMeta{
		AgentID:       "agent-123",
		Name:          "MyProject",
		Mode:          "agent",
		CreatedAt:     1718000000000,
		LastUsedModel: "composer-2",
	}

	blobs := []map[string]any{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "build me a REST API"},
		{"role": "assistant", "content": "I'll help you build a REST API..."},
	}

	dbPath := createTestDB(t, sessDir, meta, blobs)

	c := &CursorAgent{baseDir: dir}
	s, err := c.scanSession(dbPath, wsHash)
	if err != nil {
		t.Fatal(err)
	}

	if s.ID != "agent-123" {
		t.Errorf("ID: got %q, want agent-123", s.ID)
	}
	if s.Provider != "cursor" {
		t.Errorf("Provider: got %q, want cursor", s.Provider)
	}
	if s.ProjectName != "MyProject" {
		t.Errorf("ProjectName: got %q, want MyProject", s.ProjectName)
	}
	if s.Model != "composer-2" {
		t.Errorf("Model: got %q, want composer-2", s.Model)
	}
	if s.MessageCount != 3 {
		t.Errorf("MessageCount: got %d, want 3", s.MessageCount)
	}
	if s.Tokens.TotalTokens == 0 {
		t.Error("expected non-zero token estimate")
	}
	if s.Created.IsZero() {
		t.Error("Created should not be zero")
	}
}

func TestScanSession_FallbackName(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "ws", "sess")
	os.MkdirAll(sessDir, 0755)

	meta := sessionMeta{
		AgentID:   "agent-456",
		Name:      "",
		CreatedAt: 1718000000000,
	}

	dbPath := createTestDB(t, sessDir, meta, nil)

	c := &CursorAgent{baseDir: dir}
	s, err := c.scanSession(dbPath, "ws")
	if err != nil {
		t.Fatal(err)
	}

	if s.ProjectName != "cursor-session" {
		t.Errorf("ProjectName: got %q, want cursor-session", s.ProjectName)
	}
}

func TestScanSession_ExtractsFirstUserPrompt(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "ws", "sess")
	os.MkdirAll(sessDir, 0755)

	meta := sessionMeta{
		AgentID:   "agent-789",
		Name:      "Test",
		CreatedAt: 1718000000000,
	}

	blobs := []map[string]any{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "fix the login bug"},
		{"role": "assistant", "content": "Sure, let me look at the code."},
	}

	dbPath := createTestDB(t, sessDir, meta, blobs)

	c := &CursorAgent{baseDir: dir}
	s, err := c.scanSession(dbPath, "ws")
	if err != nil {
		t.Fatal(err)
	}

	if s.Summary != "[cli] fix the login bug" {
		t.Errorf("Summary: got %q, want '[cli] fix the login bug'", s.Summary)
	}
}

func TestScanSession_TokenEstimation(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "ws", "sess")
	os.MkdirAll(sessDir, 0755)

	meta := sessionMeta{
		AgentID:   "agent-tok",
		Name:      "TokenTest",
		CreatedAt: 1718000000000,
	}

	blobs := []map[string]any{
		{"role": "user", "content": "short message"},
		{"role": "assistant", "content": "another short response here"},
	}

	dbPath := createTestDB(t, sessDir, meta, blobs)

	c := &CursorAgent{baseDir: dir}
	s, err := c.scanSession(dbPath, "ws")
	if err != nil {
		t.Fatal(err)
	}

	if s.Tokens.InputTokens == 0 {
		t.Error("expected non-zero InputTokens")
	}
	if s.Tokens.OutputTokens == 0 {
		t.Error("expected non-zero OutputTokens")
	}
	if s.Tokens.TotalTokens != s.Tokens.InputTokens+s.Tokens.OutputTokens {
		t.Errorf("TotalTokens %d != InputTokens %d + OutputTokens %d",
			s.Tokens.TotalTokens, s.Tokens.InputTokens, s.Tokens.OutputTokens)
	}
}

func TestExtractProjectPath_Found(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	projectDir := t.TempDir()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("CREATE TABLE blobs (id TEXT PRIMARY KEY, data BLOB)")

	systemPrompt := map[string]any{
		"role":    "system",
		"content": "You are a helpful assistant.\nWorkspace Path: " + projectDir + "\nFollow instructions.",
	}
	data, _ := json.Marshal(systemPrompt)
	db.Exec("INSERT INTO blobs (id, data) VALUES ('sys', ?)", data)
	db.Close()

	db2, _ := sql.Open("sqlite", dbPath+"?mode=ro")
	defer db2.Close()

	got := extractProjectPath(db2)
	if got != projectDir {
		t.Errorf("extractProjectPath() = %q, want %q", got, projectDir)
	}
}

func TestExtractProjectPath_NotFound(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("CREATE TABLE blobs (id TEXT PRIMARY KEY, data BLOB)")

	msg := map[string]any{"role": "user", "content": "hello"}
	data, _ := json.Marshal(msg)
	db.Exec("INSERT INTO blobs (id, data) VALUES ('u1', ?)", data)
	db.Close()

	db2, _ := sql.Open("sqlite", dbPath+"?mode=ro")
	defer db2.Close()

	got := extractProjectPath(db2)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestIsSystemContent(t *testing.T) {
	tests := []struct {
		input any
		want  bool
	}{
		{"<user_info>some context</user_info>", true},
		{"You are a helpful assistant", true},
		{"fix the login page", false},
		{42, false},
		{nil, false},
	}

	for _, tt := range tests {
		got := isSystemContent(tt.input)
		if got != tt.want {
			t.Errorf("isSystemContent(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestCleanPrompt_StringContent(t *testing.T) {
	got := cleanPrompt("fix the login page")
	if got != "fix the login page" {
		t.Errorf("got %q, want 'fix the login page'", got)
	}
}

func TestCleanPrompt_ArrayContent(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "build a dashboard"},
	}
	got := cleanPrompt(content)
	if got != "build a dashboard" {
		t.Errorf("got %q, want 'build a dashboard'", got)
	}
}

func TestCleanPrompt_StripsTags(t *testing.T) {
	got := cleanPrompt("<user_query>hello world</user_query>")
	if got != "hello world" {
		t.Errorf("got %q, want 'hello world'", got)
	}
}

func TestCleanPrompt_Truncation(t *testing.T) {
	long := string(make([]byte, 200))
	got := cleanPrompt(long)
	if len(got) != 150 {
		t.Errorf("expected length 150, got %d", len(got))
	}
}

func TestResumeCommand_CLI(t *testing.T) {
	c := &CursorAgent{}
	cmd, args := c.ResumeCommand(provider.Session{
		ID:          "agent-123",
		Summary:     "[cli] some prompt",
		ProjectPath: "/home/user/project",
	})
	if cmd != "agent" {
		t.Errorf("cmd: got %q, want agent", cmd)
	}
	if len(args) != 4 || args[0] != "--resume" || args[1] != "agent-123" || args[2] != "--workspace" || args[3] != "/home/user/project" {
		t.Errorf("args: got %v, want [--resume agent-123 --workspace /home/user/project]", args)
	}
}

func TestResumeCommand_CLI_NoPath(t *testing.T) {
	c := &CursorAgent{}
	cmd, args := c.ResumeCommand(provider.Session{
		ID:      "agent-456",
		Summary: "[cli] some prompt",
	})
	if cmd != "agent" {
		t.Errorf("cmd: got %q, want agent", cmd)
	}
	if len(args) != 2 || args[0] != "--resume" || args[1] != "agent-456" {
		t.Errorf("args: got %v, want [--resume agent-456]", args)
	}
}

func TestResumeCommand_IDE(t *testing.T) {
	c := &CursorAgent{}
	cmd, args := c.ResumeCommand(provider.Session{
		Summary:     "[ide:agent] My Task",
		ProjectPath: "/home/user/project",
	})
	if cmd != "cursor" {
		t.Errorf("cmd: got %q, want cursor", cmd)
	}
	if len(args) != 1 || args[0] != "/home/user/project" {
		t.Errorf("args: got %v, want [/home/user/project]", args)
	}
}

func TestResumeCommand_IDE_NoPath(t *testing.T) {
	c := &CursorAgent{}
	cmd, args := c.ResumeCommand(provider.Session{
		Summary: "[ide:chat] Some Chat",
	})
	if cmd != "cursor" {
		t.Errorf("cmd: got %q, want cursor", cmd)
	}
	if args != nil {
		t.Errorf("args: got %v, want nil", args)
	}
}

func TestScanSessions_Basic(t *testing.T) {
	dir := t.TempDir()

	// Create a workspace dir with one session
	wsDir := filepath.Join(dir, "workspace-hash")
	sessDir := filepath.Join(wsDir, "session-uuid")
	os.MkdirAll(sessDir, 0755)

	meta := sessionMeta{
		AgentID:       "agent-scan",
		Name:          "ScanTest",
		CreatedAt:     1718000000000,
		LastUsedModel: "composer-2-fast",
	}
	blobs := []map[string]any{
		{"role": "user", "content": "hello"},
	}
	createTestDB(t, sessDir, meta, blobs)

	c := &CursorAgent{baseDir: dir}
	sessions, err := c.ScanSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// May include IDE sessions too if the test machine has Cursor installed,
	// so just verify our CLI session is in there
	var found bool
	for _, s := range sessions {
		if s.ID == "agent-scan" {
			found = true
			if s.Provider != "cursor" {
				t.Errorf("Provider: got %q, want cursor", s.Provider)
			}
			if s.Model != "composer-2-fast" {
				t.Errorf("Model: got %q, want composer-2-fast", s.Model)
			}
			break
		}
	}
	if !found {
		t.Error("expected to find session agent-scan in results")
	}
}

func TestScanSessions_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	c := &CursorAgent{baseDir: dir}
	sessions, err := c.ScanSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// May have IDE sessions from the actual system, but CLI sessions should be 0
	for _, s := range sessions {
		if s.Summary != "" && s.Summary[0] == '[' && len(s.Summary) > 4 && s.Summary[1:4] == "cli" {
			t.Error("expected no CLI sessions from empty base dir")
		}
	}
}

func TestScanSessions_MissingDir(t *testing.T) {
	c := &CursorAgent{baseDir: "/nonexistent/cursor/chats"}
	_, err := c.ScanSessions(context.Background())
	if err == nil {
		t.Error("expected error for missing directory")
	}
}

func TestNewCommand(t *testing.T) {
	c := &CursorAgent{}
	cmd, args := c.NewCommand("/tmp/project")
	if cmd != "agent" {
		t.Errorf("cmd: got %q, want agent", cmd)
	}
	if len(args) != 1 || args[0] != "/tmp/project" {
		t.Errorf("args: got %v, want [/tmp/project]", args)
	}
}
