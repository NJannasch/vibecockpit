package claudedesktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"vibecockpit/internal/provider"
)

type ClaudeDesktop struct{}

func New() *ClaudeDesktop { return &ClaudeDesktop{} }

func (c *ClaudeDesktop) Name() string { return "claude-desktop" }
func (c *ClaudeDesktop) Icon() string { return "⬢" }

// Available reports whether Claude Desktop is installed on this machine.
// Claude Desktop only ships for macOS and Windows; we currently only
// support macOS where the on-disk layout under Application Support is
// well understood.
func Available() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	dir, err := sessionsDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(dir)
	return err == nil
}

func sessionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "Claude", "claude-code-sessions"), nil
}

// CliSessionIDs returns the set of cliSessionIds referenced by Claude
// Desktop wrappers. The standalone claude provider uses this to skip
// JSONL transcripts that are surfaced via claude-desktop, avoiding
// double-counting in the dashboard. Safe to call on any platform — the
// walk silently returns nothing when the sessions directory is missing.
func CliSessionIDs() map[string]struct{} {
	out := map[string]struct{}{}
	dir, err := sessionsDir()
	if err != nil {
		return out
	}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		w, err := readWrapper(path)
		if err != nil || w.CliSessionID == "" {
			return nil
		}
		out[w.CliSessionID] = struct{}{}
		return nil
	})
	return out
}

type wrapper struct {
	SessionID       string `json:"sessionId"`
	CliSessionID    string `json:"cliSessionId"`
	Cwd             string `json:"cwd"`
	OriginCwd       string `json:"originCwd"`
	CreatedAt       int64  `json:"createdAt"`
	LastActivityAt  int64  `json:"lastActivityAt"`
	Model           string `json:"model"`
	Title           string `json:"title"`
	IsArchived      bool   `json:"isArchived"`
	CompletedTurns  int    `json:"completedTurns"`
	PermissionMode  string `json:"permissionMode"`
}

func readWrapper(path string) (wrapper, error) {
	var w wrapper
	data, err := os.ReadFile(path)
	if err != nil {
		return w, err
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return w, err
	}
	return w, nil
}

func (c *ClaudeDesktop) ScanSessions(_ context.Context) ([]provider.Session, error) {
	dir, err := sessionsDir()
	if err != nil {
		return nil, err
	}

	// Collect wrappers, then dedupe by cliSessionId. When multiple wrappers
	// share a cliSessionId (which happens after Claude Desktop's `claude://resume`
	// import creates a shadow wrapper), keep the one with the richest
	// metadata: a non-empty title wins over a blank one; ties broken by
	// earliest CreatedAt.
	groups := map[string][]wrapperEntry{} // keyed by cliSessionId; "" entries get unique keys
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		w, err := readWrapper(path)
		if err != nil || w.SessionID == "" {
			return nil
		}
		// Stub wrappers — Claude Desktop pre-creates these when the user
		// opens the "new session" UI but never sends a message. They
		// have no CLI transcript and no completed turns; surfacing them
		// just clutters the dashboard.
		if w.CliSessionID == "" && w.CompletedTurns == 0 {
			return nil
		}
		key := w.CliSessionID
		if key == "" {
			key = "__nocli__:" + w.SessionID
		}
		groups[key] = append(groups[key], wrapperEntry{path: path, w: w})
		return nil
	})

	var sessions []provider.Session
	for _, group := range groups {
		canonical := pickCanonical(group)

		// Latest activity across the group reflects when the user last
		// touched the underlying conversation, regardless of which shadow
		// wrapper recorded it.
		latest := canonical.w.LastActivityAt
		for _, e := range group {
			if e.w.LastActivityAt > latest {
				latest = e.w.LastActivityAt
			}
		}

		w := canonical.w
		s := provider.Session{
			ID:           w.SessionID,
			Provider:     "claude-desktop",
			ProjectName:  filepath.Base(w.Cwd),
			ProjectPath:  w.Cwd,
			Summary:      w.Title,
			Model:        w.Model,
			MessageCount: w.CompletedTurns,
			Created:      msToTime(w.CreatedAt),
			Modified:     msToTime(latest),
			DataPath:     canonical.path,
		}
		sessions = append(sessions, s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})
	return sessions, nil
}

type wrapperEntry struct {
	path string
	w    wrapper
}

func pickCanonical(group []wrapperEntry) wrapperEntry {
	best := group[0]
	for _, e := range group[1:] {
		if best.w.Title == "" && e.w.Title != "" {
			best = e
			continue
		}
		if e.w.Title == "" {
			continue
		}
		if e.w.CreatedAt > 0 && (best.w.CreatedAt == 0 || e.w.CreatedAt < best.w.CreatedAt) {
			best = e
		}
	}
	return best
}

func msToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

func (c *ClaudeDesktop) ResumeCommand(s provider.Session) (string, []string) {
	// The session's ID is the wrapper id (local_<uuid>); we re-read the
	// wrapper from DataPath to get the cliSessionId since the deep link
	// requires the underlying CLI UUID, not the wrapper id.
	cli := lookupCliSessionID(s)
	if cli == "" {
		// GUI-only session with no transcript yet — nothing to resume,
		// just open Claude Desktop and let the user pick.
		return "open", []string{"-a", "Claude"}
	}
	return "open", []string{fmt.Sprintf("claude://resume?session=%s", cli)}
}

func lookupCliSessionID(s provider.Session) string {
	if s.DataPath != "" {
		if w, err := readWrapper(s.DataPath); err == nil {
			return w.CliSessionID
		}
	}
	// Fallback: glob for the wrapper file. Used when DataPath is unset
	// (e.g., after a round-trip through the JSON API).
	dir, err := sessionsDir()
	if err != nil {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*", "*", s.ID+".json"))
	for _, m := range matches {
		if w, err := readWrapper(m); err == nil && w.CliSessionID != "" {
			return w.CliSessionID
		}
	}
	return ""
}

func (c *ClaudeDesktop) NewCommand(dir string) (string, []string) {
	q := url.Values{}
	q.Set("folder", dir)
	return "open", []string{"claude://code/new?" + q.Encode()}
}

func (c *ClaudeDesktop) DeleteSession(sessionID string) error {
	dir, err := sessionsDir()
	if err != nil {
		return err
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*", "*", sessionID+".json"))
	if len(matches) == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			return err
		}
	}
	return nil
}
