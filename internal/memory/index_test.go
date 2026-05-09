package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vibecockpit/internal/provider"
)

// newRawSQLite is a test-only helper that creates a SQLite file with a
// user_version PRAGMA but none of the FTS5 tables — used to simulate a
// pre-v3 export so we can verify Import refuses to touch it.
func newRawSQLite(path string, version int) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func newTestIndex(t *testing.T) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	idx, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func sessionDoc(id string, msgs ...string) SessionDoc {
	doc := SessionDoc{
		ID:          id,
		Provider:    "claude",
		ProjectName: "webapp",
		ProjectPath: "/Users/x/webapp",
		Model:       "claude-sonnet-4-6",
		GitBranch:   "main",
		Modified:    time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Build the auth flow",
	}
	for i, m := range msgs {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		doc.Messages = append(doc.Messages, Message{Idx: i, Role: role, Content: m})
	}
	return doc
}

func TestPutAndSearch_RoundTrip(t *testing.T) {
	idx := newTestIndex(t)

	indexed, err := idx.PutSession(sessionDoc("s1",
		"how do I fix the JWT validation bug?",
		"check the middleware.",
	))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if !indexed {
		t.Fatal("expected first PutSession to actually index")
	}

	results, err := idx.Search("jwt", SearchOpts{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(results))
	}
	r := results[0]
	if r.SessionID != "s1" {
		t.Errorf("unexpected session id: %q", r.SessionID)
	}
	if r.MessageCount != 2 {
		t.Errorf("expected MessageCount=2, got %d", r.MessageCount)
	}
	if r.MessageIdx != 0 || r.MessageRole != "user" {
		t.Errorf("expected hit on first user msg, got idx=%d role=%q", r.MessageIdx, r.MessageRole)
	}
	if !strings.Contains(r.Snippet, "<mark>") {
		t.Errorf("expected <mark> highlighting in snippet, got %q", r.Snippet)
	}
}

func TestPut_SkipsUnchangedContent(t *testing.T) {
	idx := newTestIndex(t)
	doc := sessionDoc("s1", "content here", "and more")

	indexed, _ := idx.PutSession(doc)
	if !indexed {
		t.Fatal("first PutSession should index")
	}
	indexed, _ = idx.PutSession(doc)
	if indexed {
		t.Fatal("re-PutSession with identical content should be a skip")
	}
}

func TestSearch_PorterStemming(t *testing.T) {
	// Porter stems plurals/-ed/-ing to a common root. We test "running"
	// vs "run" rather than "auth" vs "authenticate" — Porter doesn't
	// collapse those to a shared stem (different rule chains), so
	// matching the short prefix requires "auth*" not bare "auth".
	idx := newTestIndex(t)
	_, _ = idx.PutSession(sessionDoc("s1", "the migrations are running smoothly"))

	results, err := idx.Search("running", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected literal match for 'running'")
	}
	results, err = idx.Search("run", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("porter stemming failed: 'run' should match 'running'")
	}
}

func TestSearch_PrefixMatch(t *testing.T) {
	idx := newTestIndex(t)
	_, _ = idx.PutSession(sessionDoc("s1", "authenticate the request properly"))

	// "auth*" is the FTS5 way to match a prefix when stemming wouldn't.
	got, err := idx.Search("auth*", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Errorf("prefix search 'auth*' should match 'authenticate'")
	}
}

func TestSearch_ProviderFilter(t *testing.T) {
	idx := newTestIndex(t)

	_, _ = idx.PutSession(SessionDoc{ID: "c1", Provider: "claude", Messages: []Message{{Idx: 0, Role: "user", Content: "fix the auth bug"}}})
	_, _ = idx.PutSession(SessionDoc{ID: "x1", Provider: "cursor", Messages: []Message{{Idx: 0, Role: "user", Content: "fix the auth bug"}}})

	got, err := idx.Search("auth", SearchOpts{Providers: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != "c1" {
		t.Errorf("provider filter failed; got %+v", got)
	}
}

func TestSearch_DedupesPerSession(t *testing.T) {
	// One session with multiple matching messages should produce a
	// single Result. Which message wins is up to BM25 — we only
	// assert that *some* message in the session anchors the result
	// and that it isn't a non-matching one.
	idx := newTestIndex(t)
	_, _ = idx.PutSession(sessionDoc("s1",
		"a quick auth note",
		"more on auth and the validation flow",
		"unrelated text",
		"talking about auth in this final message",
	))

	results, err := idx.Search("auth", SearchOpts{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected dedupe to 1 result for one session, got %d", len(results))
	}
	matching := map[int]bool{0: true, 1: true, 3: true}
	if !matching[results[0].MessageIdx] {
		t.Errorf("anchor message should be one that matches; got idx %d (the unrelated message)", results[0].MessageIdx)
	}
}

func TestRemove_DropsFromIndex(t *testing.T) {
	idx := newTestIndex(t)
	_, _ = idx.PutSession(sessionDoc("s1", "auth bug"))

	if err := idx.Remove("s1"); err != nil {
		t.Fatal(err)
	}
	got, _ := idx.Search("auth", SearchOpts{})
	if len(got) != 0 {
		t.Errorf("expected 0 results after Remove, got %d", len(got))
	}
}

func TestDelete_WritesTombstoneWithMetadata(t *testing.T) {
	idx := newTestIndex(t)
	doc := sessionDoc("s1", "auth bug")
	doc.Host = "macbook"
	_, _ = idx.PutSession(doc)

	if err := idx.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	got, _ := idx.Search("auth", SearchOpts{})
	if len(got) != 0 {
		t.Errorf("expected 0 search results after Delete, got %d", len(got))
	}
	tombstoned, err := idx.IsTombstoned("s1")
	if err != nil {
		t.Fatal(err)
	}
	if !tombstoned {
		t.Fatal("expected s1 to be tombstoned")
	}
	list, err := idx.ListTombstones()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 tombstone, got %d", len(list))
	}
	tomb := list[0]
	if tomb.SessionID != "s1" || tomb.Provider != "claude" || tomb.ProjectName != "webapp" || tomb.Summary != "Build the auth flow" || tomb.Host != "macbook" {
		t.Errorf("tombstone metadata not snapshotted correctly: %+v", tomb)
	}
	if tomb.DeletedAt.IsZero() {
		t.Error("tombstone DeletedAt was zero")
	}
}

func TestUntombstone_AllowsReImport(t *testing.T) {
	src := newTestIndex(t)
	_, _ = src.PutSession(sessionDoc("s1", "comes back later"))

	exportPath := filepath.Join(t.TempDir(), "src.db")
	if err := src.Export(exportPath); err != nil {
		t.Fatal(err)
	}

	dst := newTestIndex(t)
	_, _ = dst.PutSession(sessionDoc("s1", "comes back later"))
	if err := dst.Delete("s1"); err != nil {
		t.Fatal(err)
	}

	added, err := dst.Import(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("tombstoned session should be skipped, got %d added", added)
	}

	if err := dst.Untombstone("s1"); err != nil {
		t.Fatal(err)
	}
	added, err = dst.Import(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Errorf("after Untombstone, re-import should re-add the session, got %d", added)
	}
}

func TestImport_SkipsTombstonedSessions(t *testing.T) {
	src := newTestIndex(t)
	_, _ = src.PutSession(sessionDoc("s1", "deleted-on-other-machine"))

	exportPath := filepath.Join(t.TempDir(), "src.db")
	if err := src.Export(exportPath); err != nil {
		t.Fatal(err)
	}

	dst := newTestIndex(t)
	_, _ = dst.PutSession(sessionDoc("s1", "local copy"))
	if err := dst.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	added, err := dst.Import(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("expected tombstoned session to be skipped, got %d added", added)
	}
	got, _ := dst.Search("deleted", SearchOpts{})
	if len(got) != 0 {
		t.Error("tombstoned session was resurrected by import")
	}
}

func TestImport_IdempotentOnRepeatedImport(t *testing.T) {
	src := newTestIndex(t)
	_, _ = src.PutSession(sessionDoc("s1", "hello world", "second message"))
	_, _ = src.PutSession(sessionDoc("s2", "another session"))

	exportPath := filepath.Join(t.TempDir(), "src.db")
	if err := src.Export(exportPath); err != nil {
		t.Fatal(err)
	}

	dst := newTestIndex(t)
	added1, err := dst.Import(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if added1 != 2 {
		t.Errorf("first import: expected 2, got %d", added1)
	}
	stats1, _ := dst.Stats()

	// Re-import the same file three more times — must remain idempotent.
	for i := 0; i < 3; i++ {
		added, err := dst.Import(exportPath)
		if err != nil {
			t.Fatalf("re-import %d: %v", i, err)
		}
		if added != 0 {
			t.Errorf("re-import %d: expected 0 new sessions, got %d", i, added)
		}
	}
	stats2, _ := dst.Stats()
	if stats1.Sessions != stats2.Sessions {
		t.Errorf("session count drifted: %d → %d", stats1.Sessions, stats2.Sessions)
	}
	if stats1.Messages != stats2.Messages {
		t.Errorf("message count drifted (duplication!): %d → %d", stats1.Messages, stats2.Messages)
	}
	// Search must still return exactly one hit per session — duplicates
	// would surface as multiple identical results before dedupe.
	got, _ := dst.Search("hello", SearchOpts{Limit: 50})
	if len(got) != 1 {
		t.Errorf("expected 1 hit after idempotent re-imports, got %d", len(got))
	}
}

func TestSanitizeFTSQuery_QuotesPunctuationBarewords(t *testing.T) {
	cases := map[string]string{
		// punctuation barewords get quoted
		"example.com":          `"example.com"`,
		"v1.2.3":                `"v1.2.3"`,
		"feat/login":            `"feat/login"`,
		"user@example.com":      `"user@example.com"`,
		"auth-bug":              `"auth-bug"`,
		// safe barewords pass through
		"auth":   "auth",
		"auth1":  "auth1",
		"migrat*": "migrat*",
		// operators preserved
		"auth NOT okta":     "auth NOT okta",
		"a AND b":           "a AND b",
		"a OR b":            "a OR b",
		// already-quoted phrases preserved
		`"jwt validation"`: `"jwt validation"`,
		// mixed: only the punctuation token gets quoted
		"deploy v1.2.3 NOT alpha": `deploy "v1.2.3" NOT alpha`,
	}
	for in, want := range cases {
		got := sanitizeFTSQuery(in)
		if got != want {
			t.Errorf("sanitizeFTSQuery(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSearch_AutoQuotesPunctuationOnRetry(t *testing.T) {
	idx := newTestIndex(t)
	doc := SessionDoc{
		ID:          "s1",
		Provider:    "claude",
		ProjectName: "site",
		Modified:    time.Now().UTC(),
		Messages: []Message{
			{Idx: 0, Role: "user", Content: "deploying example.com staging"},
			{Idx: 1, Role: "assistant", Content: "switched to v1.2.3 last week"},
		},
	}
	if _, err := idx.PutSession(doc); err != nil {
		t.Fatal(err)
	}

	// Raw "example.com" would fail FTS5 parse; Search should retry
	// silently and still find the hit.
	got, err := idx.Search("example.com", SearchOpts{})
	if err != nil {
		t.Fatalf("expected auto-retry to succeed, got error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 hit for example.com, got %d", len(got))
	}

	got, err = idx.Search("v1.2.3", SearchOpts{})
	if err != nil {
		t.Fatalf("retry on v1.2.3: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 hit for v1.2.3, got %d", len(got))
	}

	// Power-user query must still work first try (not punctuation).
	got, err = idx.Search("staging NOT alpha", SearchOpts{})
	if err != nil {
		t.Fatalf("operator query: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 hit for boolean query, got %d", len(got))
	}
}

func TestImport_RejectsMismatchedSchema(t *testing.T) {
	// Build a fake "old-schema" file: an empty SQLite db with user_version 1.
	oldPath := filepath.Join(t.TempDir(), "old.db")
	old, err := newRawSQLite(oldPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Close()

	dst := newTestIndex(t)
	_, err = dst.Import(oldPath)
	if err == nil {
		t.Fatal("expected import of mismatched-schema file to fail")
	}
	if !strings.Contains(err.Error(), "schema") {
		t.Errorf("expected error to mention schema, got: %v", err)
	}
}

func TestContext_ReturnsWindowAroundCenter(t *testing.T) {
	idx := newTestIndex(t)
	doc := sessionDoc("s1",
		"msg 0", "msg 1", "msg 2", "msg 3", "msg 4", "msg 5", "msg 6", "msg 7", "msg 8",
	)
	_, _ = idx.PutSession(doc)

	ctx, err := idx.Context("s1", 4, 2) // expect msg 2..6 (5 messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx) != 5 {
		t.Fatalf("expected 5 messages in window, got %d", len(ctx))
	}
	if ctx[0].Idx != 2 || ctx[4].Idx != 6 {
		t.Errorf("window edges wrong: got %d..%d", ctx[0].Idx, ctx[4].Idx)
	}
	centerCount := 0
	for _, m := range ctx {
		if m.IsCenter {
			centerCount++
			if m.Idx != 4 {
				t.Errorf("center marked on wrong message: idx=%d", m.Idx)
			}
		}
	}
	if centerCount != 1 {
		t.Errorf("expected exactly one center message, got %d", centerCount)
	}
}

func TestContext_ClampsLowerBound(t *testing.T) {
	idx := newTestIndex(t)
	_, _ = idx.PutSession(sessionDoc("s1", "0", "1", "2", "3"))

	ctx, err := idx.Context("s1", 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	// center=0, window=3 → low clamped to 0; high=3
	if len(ctx) != 4 {
		t.Errorf("expected 4 messages, got %d", len(ctx))
	}
	if ctx[0].Idx != 0 {
		t.Errorf("expected first idx=0, got %d", ctx[0].Idx)
	}
}

func TestStats_ReportsSessionsAndMessages(t *testing.T) {
	idx := newTestIndex(t)
	_, _ = idx.PutSession(sessionDoc("s1", "a", "b", "c"))
	_, _ = idx.PutSession(sessionDoc("s2", "x", "y"))

	st, err := idx.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.Sessions != 2 {
		t.Errorf("Sessions: got %d, want 2", st.Sessions)
	}
	if st.Messages != 5 {
		t.Errorf("Messages: got %d, want 5", st.Messages)
	}
}

func TestExportImport_RoundTrip(t *testing.T) {
	src := newTestIndex(t)
	_, _ = src.PutSession(sessionDoc("s1", "hamburg booking flow"))
	_, _ = src.PutSession(sessionDoc("s2", "auth middleware refactor"))

	exportPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := src.Export(exportPath); err != nil {
		t.Fatalf("export: %v", err)
	}
	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("export file missing: %v", err)
	}

	dst := newTestIndex(t)
	added, err := dst.Import(exportPath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if added != 2 {
		t.Errorf("expected 2 sessions added, got %d", added)
	}
	got, _ := dst.Search("hamburg", SearchOpts{})
	if len(got) != 1 {
		t.Errorf("imported index missing hamburg session; got %d results", len(got))
	}
}

func TestImport_LocalWinsOnConflict(t *testing.T) {
	a := newTestIndex(t)
	_, _ = a.PutSession(SessionDoc{ID: "s1", Provider: "claude", Summary: "old", Messages: []Message{{Idx: 0, Role: "user", Content: "old content"}}})

	exportPath := filepath.Join(t.TempDir(), "old.db")
	if err := a.Export(exportPath); err != nil {
		t.Fatal(err)
	}

	b := newTestIndex(t)
	_, _ = b.PutSession(SessionDoc{ID: "s1", Provider: "claude", Summary: "fresh", Messages: []Message{{Idx: 0, Role: "user", Content: "fresh content"}}})

	added, err := b.Import(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("expected 0 new sessions (conflict), got %d", added)
	}
	got, _ := b.Search("fresh", SearchOpts{})
	if len(got) == 0 {
		t.Fatal("local content was clobbered by import — conflict resolution broken")
	}
	got, _ = b.Search("old", SearchOpts{})
	if len(got) != 0 {
		t.Errorf("imported content leaked into local index for conflicting id")
	}
}

// --- indexer / extractor wiring -----------------------------------------

type fakeProvider struct {
	name     string
	sessions []provider.Session
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Icon() string { return "?" }
func (f *fakeProvider) ScanSessions(_ context.Context) ([]provider.Session, error) {
	return f.sessions, nil
}
func (f *fakeProvider) ResumeCommand(_ provider.Session) (string, []string) { return "", nil }
func (f *fakeProvider) NewCommand(_ string) (string, []string)              { return "", nil }
func (f *fakeProvider) DeleteSession(_ string) error                        { return nil }

func writeJSONL(t *testing.T, dir, name string, lines []string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIndexer_IndexAll_ExtractsMessagesAndOrphans(t *testing.T) {
	idx := newTestIndex(t)
	dir := t.TempDir()

	jsonl := writeJSONL(t, dir, "s1.jsonl", []string{
		`{"type":"user","message":{"content":"how do I deploy the auth service?"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"set the JWT_SECRET env var and run helm upgrade."}]}}`,
		`{"type":"user","message":{"content":"thanks!"}}`,
	})
	otherJSONL := writeJSONL(t, dir, "s2.jsonl", []string{
		`{"type":"user","message":{"content":"unrelated session about colors"}}`,
	})

	fp := &fakeProvider{
		name: "claude",
		sessions: []provider.Session{
			{ID: "s1", Provider: "claude", ProjectName: "p1", DataPath: jsonl, Modified: time.Now()},
			{ID: "s2", Provider: "claude", ProjectName: "p1", DataPath: otherJSONL, Modified: time.Now()},
		},
	}

	idxer := NewIndexer(idx)
	run := idxer.IndexAll(context.Background(), []provider.Provider{fp})
	if run.Indexed != 2 {
		t.Fatalf("expected 2 indexed, got %+v", run)
	}

	st, _ := idx.Stats()
	if st.Messages != 4 {
		t.Errorf("expected 4 messages indexed (3 + 1), got %d", st.Messages)
	}

	// Search hits real transcript content; result anchors a specific message.
	got, _ := idx.Search("deploy auth", SearchOpts{})
	if len(got) == 0 {
		t.Fatal("expected to find content extracted from JSONL")
	}
	if got[0].MessageCount != 3 {
		t.Errorf("expected MessageCount=3 for s1, got %d", got[0].MessageCount)
	}

	// Drop one session from the live set; orphan cleanup should remove it.
	fp.sessions = fp.sessions[:1]
	run = idxer.IndexAll(context.Background(), []provider.Provider{fp})
	if run.Removed != 1 {
		t.Fatalf("expected 1 orphan removed, got %+v", run)
	}
	got, _ = idx.Search("colors", SearchOpts{})
	if len(got) != 0 {
		t.Errorf("orphan still in index; got %+v", got)
	}
}

func TestExtract_SanitizesSecrets(t *testing.T) {
	dir := t.TempDir()
	body := "sk-ant-api03-" + strings.Repeat("A", 93) + "AA"
	jsonl := writeJSONL(t, dir, "leak.jsonl", []string{
		`{"type":"user","message":{"content":"my key is ` + body + `"}}`,
	})
	msgs, err := extractJSONLMessages(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if strings.Contains(msgs[0].Content, body) {
		t.Errorf("sanitize did not redact secret; content still contains the raw token")
	}
}
