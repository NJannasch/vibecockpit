// Package memory implements a full-text searchable index of every
// message in every transcript across every provider. It's the
// substrate behind the web /api/memory/* endpoints, the CLI memory
// subcommand, and the MCP search_memory tool — the same FTS5 store
// can be queried by humans and by AI agents, turning the dashboard
// into cross-tool memory.
//
// Indexing granularity is per-message: each user/assistant turn is one
// FTS5 row with the session_id and a 0-based message_idx. Search
// returns the best-scoring message per session for the list view,
// while Index.Context fetches a window of surrounding messages so
// the UI can show what was said around a hit.
package memory

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// schemaVersion bumps when the DB layout is incompatible with older
// runs. We just drop the FTS table and reindex when it changes — the
// index is always cheap to rebuild from the underlying transcripts.
const schemaVersion = 3

// DefaultPath is where the index lives when no override is given.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "vibecockpit", "cache", "memory.db"), nil
}

// Index is the public handle. Safe for concurrent use; the underlying
// sql.DB pool serializes writes through a single connection while
// allowing parallel reads.
type Index struct {
	db   *sql.DB
	path string
}

// SessionDoc is one full session's worth of indexable data.
type SessionDoc struct {
	ID          string
	Provider    string
	ProjectName string
	ProjectPath string
	Model       string
	GitBranch   string
	Host        string // machine that originated the session (set by indexer)
	Modified    time.Time
	Summary     string
	Messages    []Message
}

// Result is one search hit. The list view dedupes to the best message
// per session; MessageIdx anchors a follow-up Context() call.
type Result struct {
	SessionID    string    `json:"sessionId"`
	Provider     string    `json:"provider"`
	ProjectName  string    `json:"projectName"`
	ProjectPath  string    `json:"projectPath,omitempty"`
	Model        string    `json:"model,omitempty"`
	GitBranch    string    `json:"gitBranch,omitempty"`
	Host         string    `json:"host,omitempty"`
	Modified     time.Time `json:"modified,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	MessageIdx   int       `json:"messageIdx"`
	MessageRole  string    `json:"messageRole,omitempty"`
	MessageCount int       `json:"messageCount"`
	Snippet      string    `json:"snippet,omitempty"`
	Score        float64   `json:"score"`
}

// SearchOpts shapes a Search call. All fields are optional.
type SearchOpts struct {
	Limit       int      // default 20
	Providers   []string // exact match against the provider column
	ProjectLike string   // SQL LIKE pattern against project_name (% allowed)
	Since       time.Time
	Until       time.Time
}

// ContextMessage is one message returned from Index.Context.
type ContextMessage struct {
	Idx       int       `json:"idx"`
	Role      string    `json:"role"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	Content   string    `json:"content"`
	IsCenter  bool      `json:"isCenter"`
}

// Stats reports document/byte counts.
type Stats struct {
	Sessions    int    `json:"sessions"`
	Messages    int    `json:"messages"`
	BytesOnDisk int64  `json:"bytesOnDisk"`
	Path        string `json:"path"`
}

// Tombstone is one row of the "excluded" list. Snapshotted at delete
// time so the UI can render a useful label without joining against
// session_meta (which has already been wiped for this id).
type Tombstone struct {
	SessionID   string    `json:"sessionId"`
	DeletedAt   time.Time `json:"deletedAt"`
	Provider    string    `json:"provider,omitempty"`
	ProjectName string    `json:"projectName,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	Host        string    `json:"host,omitempty"`
}

// Open opens or creates the FTS5 index at path. Schema is applied
// idempotently. If the on-disk schema version is older than the
// current code, the FTS table is dropped and a reindex will rebuild
// it on the next IndexAll call.
func Open(path string) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir cache: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	idx := &Index{db: db, path: path}
	if err := idx.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return idx, nil
}

func (i *Index) Close() error { return i.db.Close() }

// Path returns the on-disk path to the SQLite file (used by Export).
func (i *Index) Path() string { return i.path }

func (i *Index) migrate() error {
	var current int
	_ = i.db.QueryRow(`PRAGMA user_version`).Scan(&current)

	if current < schemaVersion {
		// Drop everything FTS-related; session_meta would also need
		// reset because content_hash is keyed on the new layout.
		drop := []string{
			`DROP TABLE IF EXISTS message_fts`,
			`DROP TABLE IF EXISTS session_fts`,        // historical name from v1
			`DROP TABLE IF EXISTS session_tombstones`, // historical from a v3 prerelease
			`DROP TABLE IF EXISTS session_meta`,
		}
		for _, s := range drop {
			if _, err := i.db.Exec(s); err != nil {
				return fmt.Errorf("migrate drop: %w", err)
			}
		}
	}

	stmts := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		// Per-message rows. Porter stemmer means "auth" matches "authenticate".
		`CREATE VIRTUAL TABLE IF NOT EXISTS message_fts USING fts5(
			session_id    UNINDEXED,
			message_idx   UNINDEXED,
			role          UNINDEXED,
			timestamp     UNINDEXED,
			provider      UNINDEXED,
			project_name  UNINDEXED,
			project_path  UNINDEXED,
			model         UNINDEXED,
			git_branch    UNINDEXED,
			host          UNINDEXED,
			modified      UNINDEXED,
			content,
			tokenize='porter unicode61'
		)`,
		`CREATE TABLE IF NOT EXISTS session_meta (
			session_id    TEXT PRIMARY KEY,
			content_hash  TEXT NOT NULL,
			indexed_at    TEXT NOT NULL,
			message_count INTEGER NOT NULL DEFAULT 0,
			summary       TEXT NOT NULL DEFAULT '',
			provider      TEXT NOT NULL DEFAULT '',
			project_name  TEXT NOT NULL DEFAULT '',
			project_path  TEXT NOT NULL DEFAULT '',
			model         TEXT NOT NULL DEFAULT '',
			git_branch    TEXT NOT NULL DEFAULT '',
			host          TEXT NOT NULL DEFAULT '',
			modified      TEXT NOT NULL DEFAULT ''
		)`,
		// Tombstones make Delete() sticky across re-indexes and imports.
		// Metadata is snapshotted at delete time so the "Excluded" UI can
		// label each row even after session_meta has been wiped.
		`CREATE TABLE IF NOT EXISTS session_tombstones (
			session_id   TEXT PRIMARY KEY,
			deleted_at   TEXT NOT NULL,
			provider     TEXT NOT NULL DEFAULT '',
			project_name TEXT NOT NULL DEFAULT '',
			summary      TEXT NOT NULL DEFAULT '',
			host         TEXT NOT NULL DEFAULT ''
		)`,
		fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion),
	}
	for _, s := range stmts {
		if _, err := i.db.Exec(s); err != nil {
			return fmt.Errorf("migrate: %s: %w", firstLine(s), err)
		}
	}
	return nil
}

// PutSession writes (or replaces) a session and all its messages.
// Returns true if the session was actually re-indexed; returns false
// when the existing row's content hash matches and we skipped the
// rewrite. The hash check is what makes repeat scans cheap.
func (i *Index) PutSession(d SessionDoc) (bool, error) {
	hash := sessionContentHash(d)
	var current string
	row := i.db.QueryRow(`SELECT content_hash FROM session_meta WHERE session_id = ?`, d.ID)
	if err := row.Scan(&current); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if current == hash {
		return false, nil
	}

	tx, err := i.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM message_fts WHERE session_id = ?`, d.ID); err != nil {
		return false, err
	}

	modifiedStr := d.Modified.UTC().Format(time.RFC3339)
	for _, m := range d.Messages {
		ts := ""
		if !m.Timestamp.IsZero() {
			ts = m.Timestamp.UTC().Format(time.RFC3339)
		}
		if _, err := tx.Exec(`INSERT INTO message_fts
			(session_id, message_idx, role, timestamp, provider, project_name, project_path, model, git_branch, host, modified, content)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.ID, m.Idx, m.Role, ts, d.Provider, d.ProjectName, d.ProjectPath,
			d.Model, d.GitBranch, d.Host, modifiedStr, m.Content,
		); err != nil {
			return false, err
		}
	}

	if _, err := tx.Exec(`INSERT OR REPLACE INTO session_meta
		(session_id, content_hash, indexed_at, message_count, summary, provider, project_name, project_path, model, git_branch, host, modified)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, hash, time.Now().UTC().Format(time.RFC3339), len(d.Messages),
		d.Summary, d.Provider, d.ProjectName, d.ProjectPath, d.Model, d.GitBranch, d.Host, modifiedStr,
	); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// Remove drops a session (and all its messages) from the index without
// leaving a tombstone. Used for orphan cleanup of *local* sessions
// whose underlying transcript has disappeared from disk — those will
// simply not be re-discovered, so a tombstone would be redundant.
func (i *Index) Remove(id string) error {
	tx, err := i.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM message_fts WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM session_meta WHERE session_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete is the user-facing delete: removes the session AND records a
// tombstone with snapshotted metadata. Indexer + Import consult the
// tombstone table to refuse re-adding it. Restore via Untombstone.
func (i *Index) Delete(id string) error {
	// Snapshot metadata for the Excluded view BEFORE we wipe session_meta.
	var provider, project, summary, host string
	row := i.db.QueryRow(
		`SELECT provider, project_name, summary, host FROM session_meta WHERE session_id = ?`, id,
	)
	if err := row.Scan(&provider, &project, &summary, &host); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	tx, err := i.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM message_fts WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM session_meta WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO session_tombstones (session_id, deleted_at, provider, project_name, summary, host) VALUES (?, ?, ?, ?, ?, ?)`,
		id, time.Now().UTC().Format(time.RFC3339), provider, project, summary, host,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// Untombstone removes a tombstone — the session will be re-indexed on
// the next IndexAll (if its transcript is still on disk) or come back
// on the next Import that contains it.
func (i *Index) Untombstone(id string) error {
	_, err := i.db.Exec(`DELETE FROM session_tombstones WHERE session_id = ?`, id)
	return err
}

// IsTombstoned reports whether a session_id was previously deleted.
func (i *Index) IsTombstoned(id string) (bool, error) {
	var n int
	err := i.db.QueryRow(`SELECT COUNT(*) FROM session_tombstones WHERE session_id = ?`, id).Scan(&n)
	return n > 0, err
}

// TombstonedIDs returns the set of tombstoned ids — used by IndexAll
// and Import to filter out deleted sessions in bulk.
func (i *Index) TombstonedIDs() (map[string]struct{}, error) {
	rows, err := i.db.Query(`SELECT session_id FROM session_tombstones`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// ListTombstones returns every tombstone with its snapshotted metadata,
// most-recently-deleted first. Powers the "Excluded" view in the UI.
func (i *Index) ListTombstones() ([]Tombstone, error) {
	rows, err := i.db.Query(`
		SELECT session_id, deleted_at, provider, project_name, summary, host
		FROM session_tombstones
		ORDER BY deleted_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tombstone
	for rows.Next() {
		var t Tombstone
		var deletedAt string
		if err := rows.Scan(&t.SessionID, &deletedAt, &t.Provider, &t.ProjectName, &t.Summary, &t.Host); err != nil {
			return nil, err
		}
		if ts, err := time.Parse(time.RFC3339, deletedAt); err == nil {
			t.DeletedAt = ts
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Search runs an FTS5 MATCH query and dedupes hits to the best
// message per session — that keeps the list view tidy while still
// pointing at the right anchor for context lookup. Empty query
// returns nothing.
//
// Casual queries like "example.com" or "v1.2.3" don't parse as FTS5 by
// default because the dot is a syntax token. We try the raw query first
// (so power-user expressions like "auth NOT okta" or "(a OR b)" keep
// working as written), and only on a syntax error retry with a sanitized
// version that auto-quotes punctuation-bearing barewords. The retry is
// invisible to the caller.
func (i *Index) Search(query string, opts SearchOpts) ([]Result, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	out, err := i.searchOnce(q, opts)
	if err != nil && isFTSSyntaxError(err) {
		if sanitized := sanitizeFTSQuery(q); sanitized != q {
			return i.searchOnce(sanitized, opts)
		}
	}
	return out, err
}

// searchOnce does the actual SQL work; Search is a thin wrapper that
// adds the syntax-error retry. Refactored out so both attempts share
// the same code path.
func (i *Index) searchOnce(q string, opts SearchOpts) ([]Result, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	args := []any{q}
	where := []string{"message_fts MATCH ?"}

	if len(opts.Providers) > 0 {
		placeholders := make([]string, len(opts.Providers))
		for j, p := range opts.Providers {
			placeholders[j] = "?"
			args = append(args, p)
		}
		where = append(where, fmt.Sprintf("provider IN (%s)", strings.Join(placeholders, ",")))
	}
	if opts.ProjectLike != "" {
		where = append(where, "project_name LIKE ?")
		args = append(args, opts.ProjectLike)
	}
	if !opts.Since.IsZero() {
		where = append(where, "modified >= ?")
		args = append(args, opts.Since.UTC().Format(time.RFC3339))
	}
	if !opts.Until.IsZero() {
		where = append(where, "modified <= ?")
		args = append(args, opts.Until.UTC().Format(time.RFC3339))
	}
	// Pull more rows than we'll return: we need enough candidates to
	// dedupe down to opts.Limit unique sessions while still showing
	// the highest-scoring message per session.
	candidateLimit := opts.Limit * 6
	if candidateLimit < 60 {
		candidateLimit = 60
	}
	args = append(args, candidateLimit)

	// content is column index 11 (0-based) in the virtual table after the
	// host column was inserted between git_branch and modified in v3.
	//
	// gosec G201 false positive: the only piece formatted into the SQL
	// here is `where`, which is built above from a fixed set of literal
	// fragments ("message_fts MATCH ?", "provider IN (...)", etc.). All
	// user-controlled values stay in `args` and reach the driver via ?
	// placeholders.
	stmt := fmt.Sprintf(`
		SELECT session_id, message_idx, role, provider, project_name, project_path,
		       model, git_branch, host, modified,
		       snippet(message_fts, 11, '<mark>', '</mark>', '…', 14) AS snippet,
		       bm25(message_fts) AS score
		FROM message_fts
		WHERE %s
		ORDER BY score
		LIMIT ?`, strings.Join(where, " AND ")) //nolint:gosec // see comment above; user values are parameterized

	// Important: we hold a single-conn pool (see Open), so we must NOT
	// open a second query while these rows are still live — that would
	// deadlock. Drain into out first, then close, then enrich.
	rows, err := i.db.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []Result
	for rows.Next() {
		var r Result
		var modifiedStr string
		if err := rows.Scan(
			&r.SessionID, &r.MessageIdx, &r.MessageRole,
			&r.Provider, &r.ProjectName, &r.ProjectPath,
			&r.Model, &r.GitBranch, &r.Host, &modifiedStr, &r.Snippet, &r.Score,
		); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if seen[r.SessionID] {
			continue
		}
		seen[r.SessionID] = true
		if t, err := time.Parse(time.RFC3339, modifiedStr); err == nil {
			r.Modified = t
		}
		out = append(out, r)
		if len(out) >= opts.Limit {
			break
		}
	}
	scanErr := rows.Err()
	_ = rows.Close() // release the conn before enrichment query
	if scanErr != nil {
		return nil, scanErr
	}
	if len(out) == 0 {
		return out, nil
	}

	// Enrich with summary + message_count from session_meta in a single
	// follow-up query (rows above are now closed).
	placeholders := make([]string, len(out))
	idArgs := make([]any, len(out))
	for j, r := range out {
		placeholders[j] = "?"
		idArgs[j] = r.SessionID
	}
	mrows, err := i.db.Query(
		fmt.Sprintf(`SELECT session_id, summary, message_count FROM session_meta WHERE session_id IN (%s)`,
			strings.Join(placeholders, ",")),
		idArgs...,
	)
	if err != nil {
		return out, nil // enrichment is best-effort
	}
	meta := map[string]struct {
		summary string
		count   int
	}{}
	for mrows.Next() {
		var id, sum string
		var cnt int
		if err := mrows.Scan(&id, &sum, &cnt); err == nil {
			meta[id] = struct {
				summary string
				count   int
			}{sum, cnt}
		}
	}
	_ = mrows.Close()
	for j := range out {
		if m, ok := meta[out[j].SessionID]; ok {
			out[j].Summary = m.summary
			out[j].MessageCount = m.count
		}
	}
	return out, nil
}

// Context returns up to (2*window+1) messages centered on `center`
// for sessionID. Useful for "show me what was said around the hit".
// IsCenter is set on the row whose Idx == center.
func (i *Index) Context(sessionID string, center, window int) ([]ContextMessage, error) {
	if window < 0 {
		window = 0
	}
	if window > 50 {
		window = 50
	}
	low := center - window
	if low < 0 {
		low = 0
	}
	high := center + window

	rows, err := i.db.Query(`
		SELECT message_idx, role, timestamp, content
		FROM message_fts
		WHERE session_id = ? AND message_idx BETWEEN ? AND ?
		ORDER BY message_idx`,
		sessionID, low, high,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContextMessage
	for rows.Next() {
		var m ContextMessage
		var ts string
		if err := rows.Scan(&m.Idx, &m.Role, &ts, &m.Content); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			m.Timestamp = t
		}
		m.IsCenter = (m.Idx == center)
		out = append(out, m)
	}
	return out, rows.Err()
}

// Stats reports counts and disk size.
func (i *Index) Stats() (Stats, error) {
	st := Stats{Path: i.path}
	if err := i.db.QueryRow(`SELECT COUNT(*) FROM session_meta`).Scan(&st.Sessions); err != nil {
		return st, err
	}
	if err := i.db.QueryRow(`SELECT COALESCE(SUM(message_count), 0) FROM session_meta`).Scan(&st.Messages); err != nil {
		return st, err
	}
	if fi, err := os.Stat(i.path); err == nil {
		st.BytesOnDisk = fi.Size()
	}
	return st, nil
}

// Export writes a consistent snapshot of the index to dest.
func (i *Index) Export(dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		if err := os.Remove(dest); err != nil {
			return err
		}
	}
	_, err := i.db.Exec(`VACUUM INTO ?`, dest)
	return err
}

// Import merges another memory.db file into this one.
//
// Idempotency contract:
//   - Sessions already present locally are kept as-is (local wins).
//   - Tombstoned sessions are NOT re-added.
//   - Re-importing the same file is a no-op (no message duplication).
//
// The dedupe happens at session granularity via session_meta — once a
// session_id exists locally, neither its meta row nor any of its
// message rows are touched. message_fts has no UNIQUE constraint
// (FTS5 limitation), so the WHERE filter is the only thing keeping
// messages from being inserted twice.
func (i *Index) Import(src string) (added int, err error) {
	if _, err := os.Stat(src); err != nil {
		return 0, fmt.Errorf("source: %w", err)
	}
	if _, err := i.db.Exec(`ATTACH DATABASE ? AS src`, src); err != nil {
		return 0, fmt.Errorf("attach: %w", err)
	}
	defer func() { _, _ = i.db.Exec(`DETACH DATABASE src`) }()

	// Reject older-schema sources rather than silently column-mismatching.
	// Same-major version is fine; if the user has an old export they can
	// just re-export from the source machine after upgrading.
	var srcVersion int
	_ = i.db.QueryRow(`PRAGMA src.user_version`).Scan(&srcVersion)
	if srcVersion != schemaVersion {
		return 0, fmt.Errorf("source schema v%d does not match local v%d — re-export from the source machine", srcVersion, schemaVersion)
	}

	tx, err := i.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// Filter source: skip sessions we already have AND skip tombstoned
	// session_ids. With identical schemas, `SELECT *` lines up safely.
	if _, err := tx.Exec(`
		INSERT INTO message_fts
		SELECT * FROM src.message_fts
		WHERE session_id NOT IN (SELECT session_id FROM session_meta)
		  AND session_id NOT IN (SELECT session_id FROM session_tombstones)`); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`
		INSERT OR IGNORE INTO session_meta
		SELECT * FROM src.session_meta
		WHERE session_id NOT IN (SELECT session_id FROM session_tombstones)`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(n), nil
}

// HasSession reports whether session_id is already indexed.
func (i *Index) HasSession(id string) (bool, error) {
	var n int
	err := i.db.QueryRow(`SELECT COUNT(*) FROM session_meta WHERE session_id = ?`, id).Scan(&n)
	return n > 0, err
}

// AllSessionIDs returns every indexed session id.
func (i *Index) AllSessionIDs() ([]string, error) {
	rows, err := i.db.Query(`SELECT session_id FROM session_meta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SessionsByHost returns every session_id whose host column matches.
// Used by orphan cleanup to scope removal to *this* machine's sessions
// only — we can't see other machines' transcripts on disk, so their
// imported entries must not be treated as orphans.
func (i *Index) SessionsByHost(host string) ([]string, error) {
	rows, err := i.db.Query(`SELECT session_id FROM session_meta WHERE host = ?`, host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func sessionContentHash(d SessionDoc) string {
	h := sha256.New()
	_, _ = io.WriteString(h, d.ID)
	_, _ = io.WriteString(h, "\x00")
	_, _ = io.WriteString(h, d.Modified.UTC().Format(time.RFC3339Nano))
	_, _ = io.WriteString(h, "\x00")
	_, _ = io.WriteString(h, d.Summary)
	_, _ = io.WriteString(h, "\x00")
	for _, m := range d.Messages {
		_, _ = io.WriteString(h, m.Role)
		_, _ = io.WriteString(h, "\x00")
		_, _ = io.WriteString(h, m.Content)
		_, _ = io.WriteString(h, "\x01")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// isFTSSyntaxError reports whether err looks like an FTS5 query parser
// failure (e.g. a bareword containing ".", "-", "/" that the FTS5
// grammar can't tokenize). Used to gate the auto-quote retry path so
// genuine errors (db corruption, permission, etc.) still propagate.
func isFTSSyntaxError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "fts5: syntax error") ||
		strings.Contains(msg, "syntax error near")
}

// ftsSafeBareword matches a token that FTS5 will accept unquoted —
// alphanumeric + underscore, with an optional trailing "*" for prefix
// search. Anything outside that range needs quoting.
var ftsSafeBareword = regexp.MustCompile(`^[A-Za-z0-9_]+\*?$`)

// sanitizeFTSQuery is a "lenient retry" rewriter: it tokenizes the
// query while respecting double-quoted phrases as single atoms, then
// quotes any bareword that contains FTS5-problematic punctuation
// (., -, /, @, :, etc.) so casual queries like "example.com" or
// "v1.2.3" succeed without the user knowing about FTS5 quoting rules.
// Operator keywords (AND, OR, NOT, NEAR) and already-quoted phrases
// are passed through untouched.
//
// This is only called on the retry path after the raw query failed —
// so power-user expressions like "auth NOT okta" or "(a OR b)" still
// succeed on the first attempt with their original semantics intact.
func sanitizeFTSQuery(q string) string {
	tokens := splitFTSTokens(q)
	if len(tokens) == 0 {
		return q
	}
	for i, t := range tokens {
		switch {
		case len(t) >= 2 && t[0] == '"' && t[len(t)-1] == '"':
			// already a quoted phrase
		case t == "AND" || t == "OR" || t == "NOT" || t == "NEAR":
			// boolean operator
		case ftsSafeBareword.MatchString(t):
			// safe bareword (incl. trailing prefix *)
		default:
			// Wrap in quotes; escape any internal " by doubling, per FTS5.
			tokens[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		}
	}
	return strings.Join(tokens, " ")
}

// splitFTSTokens splits on whitespace but treats anything between
// matching double quotes as a single atom (including the surrounding
// quotes). Used by sanitizeFTSQuery so "jwt validation" survives
// retokenization as one token, not two.
func splitFTSTokens(q string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range q {
		switch {
		case r == '"':
			cur.WriteRune(r)
			inQuote = !inQuote
		case (r == ' ' || r == '\t' || r == '\n') && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
