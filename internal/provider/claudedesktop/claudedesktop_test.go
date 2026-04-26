package claudedesktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vibecockpit/internal/provider"
)

func withFakeHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

func writeWrapper(t *testing.T, root string, name string, body string) string {
	t.Helper()
	dir := filepath.Join(root, "Library", "Application Support", "Claude", "claude-code-sessions", "ws", "user")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPickCanonical_PrefersTitledOverUntitled(t *testing.T) {
	titled := wrapperEntry{
		path: "/a",
		w:    wrapper{SessionID: "local_a", Title: "Real title", CreatedAt: 200},
	}
	untitled := wrapperEntry{
		path: "/b",
		w:    wrapper{SessionID: "local_b", Title: "", CreatedAt: 100},
	}

	got := pickCanonical([]wrapperEntry{untitled, titled})
	if got.path != "/a" {
		t.Errorf("expected the titled entry to win, got %q", got.path)
	}
}

func TestPickCanonical_TieBreaksOnEarliestCreatedAt(t *testing.T) {
	older := wrapperEntry{path: "/older", w: wrapper{SessionID: "x", Title: "T", CreatedAt: 100}}
	newer := wrapperEntry{path: "/newer", w: wrapper{SessionID: "y", Title: "T", CreatedAt: 200}}

	got := pickCanonical([]wrapperEntry{newer, older})
	if got.path != "/older" {
		t.Errorf("expected oldest titled entry to win, got %q", got.path)
	}
}

func TestCliSessionIDs_CollectsNonEmpty(t *testing.T) {
	root := withFakeHome(t)

	writeWrapper(t, root, "local_aaa", `{"sessionId":"local_aaa","cliSessionId":"cli-1"}`)
	writeWrapper(t, root, "local_bbb", `{"sessionId":"local_bbb","cliSessionId":""}`)
	writeWrapper(t, root, "local_ccc", `{"sessionId":"local_ccc","cliSessionId":"cli-2"}`)

	got := CliSessionIDs()

	if _, ok := got["cli-1"]; !ok {
		t.Errorf("expected cli-1 in set; got %v", got)
	}
	if _, ok := got["cli-2"]; !ok {
		t.Errorf("expected cli-2 in set; got %v", got)
	}
	if _, ok := got[""]; ok {
		t.Errorf("empty cliSessionId should be excluded")
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries, got %d (%v)", len(got), got)
	}
}

func TestScanSessions_DedupesByCliSessionId(t *testing.T) {
	root := withFakeHome(t)

	writeWrapper(t, root, "local_orig",
		`{"sessionId":"local_orig","cliSessionId":"cli-1","title":"Hamburg website","cwd":"/Users/x/proj","createdAt":1000,"lastActivityAt":2000,"model":"claude-opus-4-7","completedTurns":3}`)
	writeWrapper(t, root, "local_shadow",
		`{"sessionId":"local_shadow","cliSessionId":"cli-1","title":"","cwd":"/Users/x/proj","createdAt":3000,"lastActivityAt":4000}`)
	// A real GUI-only session: no cliSessionId but the user actually used it.
	writeWrapper(t, root, "local_real_gui",
		`{"sessionId":"local_real_gui","title":"GUI only","cwd":"/Users/x/other","createdAt":500,"lastActivityAt":600,"completedTurns":4}`)

	c := New()
	sessions, err := c.ScanSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 deduped sessions, got %d (%+v)", len(sessions), sessions)
	}

	var hamburg, gui *provider.Session
	for i, s := range sessions {
		switch s.Summary {
		case "Hamburg website":
			hamburg = &sessions[i]
		case "GUI only":
			gui = &sessions[i]
		}
	}
	if hamburg == nil {
		t.Fatalf("did not find Hamburg session in %+v", sessions)
	}
	if gui == nil {
		t.Fatalf("did not find GUI-only session in %+v", sessions)
	}
	if hamburg.ID != "local_orig" {
		t.Errorf("expected canonical wrapper to be local_orig (titled), got %q", hamburg.ID)
	}
	if hamburg.Modified.UnixMilli() != 4000 {
		t.Errorf("expected Modified to reflect latest activity across group (4000), got %d", hamburg.Modified.UnixMilli())
	}
}

func TestScanSessions_DropsEmptyStubs(t *testing.T) {
	root := withFakeHome(t)

	writeWrapper(t, root, "local_real",
		`{"sessionId":"local_real","cliSessionId":"cli-1","title":"Real session","cwd":"/p","createdAt":1000,"lastActivityAt":2000,"completedTurns":1}`)
	// Stub: clicked "new session" in the GUI but never sent a message.
	writeWrapper(t, root, "local_stub",
		`{"sessionId":"local_stub","title":"Stub","cwd":"/p","createdAt":1500,"lastActivityAt":1500,"completedTurns":0}`)

	c := New()
	sessions, err := c.ScanSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected stubs to be filtered out, got %d sessions: %+v", len(sessions), sessions)
	}
	if sessions[0].ID != "local_real" {
		t.Errorf("expected the real session to survive; got %q", sessions[0].ID)
	}
}

func TestResumeCommand_UsesCliSessionIdFromWrapper(t *testing.T) {
	root := withFakeHome(t)
	path := writeWrapper(t, root, "local_xyz",
		`{"sessionId":"local_xyz","cliSessionId":"abc-1234"}`)

	c := New()
	bin, args := c.ResumeCommand(provider.Session{ID: "local_xyz", DataPath: path})
	if bin != "open" {
		t.Errorf("bin: got %q want open", bin)
	}
	if len(args) != 1 || !strings.Contains(args[0], "claude://resume?session=abc-1234") {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestResumeCommand_FallsBackForGuiOnly(t *testing.T) {
	c := New()
	bin, args := c.ResumeCommand(provider.Session{ID: "local_orphan"})
	if bin != "open" || len(args) != 2 || args[0] != "-a" || args[1] != "Claude" {
		t.Errorf("expected fallback to `open -a Claude`, got %q %v", bin, args)
	}
}

func TestNewCommand_ProducesCodeNewURL(t *testing.T) {
	c := New()
	bin, args := c.NewCommand("/Users/x/code")
	if bin != "open" {
		t.Errorf("bin: got %q want open", bin)
	}
	if len(args) != 1 || !strings.HasPrefix(args[0], "claude://code/new?") || !strings.Contains(args[0], "folder=") {
		t.Errorf("unexpected URL: %v", args)
	}
}
