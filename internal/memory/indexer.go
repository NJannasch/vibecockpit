package memory

import (
	"context"
	"os"
	"sync"
	"time"

	"vibecockpit/internal/provider"
)

// localHost returns the machine's hostname, falling back to "local" if
// resolution fails. Stamped on every locally-indexed SessionDoc so the
// UI can show "this came from <machine>" after import.
func localHost() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "local"
}

// Indexer drives bulk indexing. It's a thin layer around Index that
// keeps a single-flight lock so concurrent rescans don't fight each
// other for the writer connection.
type Indexer struct {
	idx *Index
	mu  sync.Mutex // serializes IndexAll runs

	lastRunMu sync.Mutex
	lastRun   IndexRun
}

// IndexRun records what the most recent indexing pass did. Surfaced via
// /api/memory/stats so users can confirm "yes, I really did re-scan".
type IndexRun struct {
	StartedAt time.Time `json:"startedAt"`
	Duration  string    `json:"duration"`
	Indexed   int       `json:"indexed"`   // sessions whose content changed
	Skipped   int       `json:"skipped"`   // hash matched, no rewrite
	Removed   int       `json:"removed"`   // orphans pruned
	Errors    int       `json:"errors"`
	LastError string    `json:"lastError,omitempty"`
}

func NewIndexer(idx *Index) *Indexer { return &Indexer{idx: idx} }

// LastRun returns a copy of the most recent run summary.
func (in *Indexer) LastRun() IndexRun {
	in.lastRunMu.Lock()
	defer in.lastRunMu.Unlock()
	return in.lastRun
}

// IndexAll walks every provider's ScanSessions, builds Docs, and writes
// them to the index. Sessions that no longer appear are removed so the
// index stays in sync with reality. Returns even if individual sessions
// fail — partial success is better than aborting.
func (in *Indexer) IndexAll(ctx context.Context, providers []provider.Provider) IndexRun {
	if !in.mu.TryLock() {
		// Another run is in flight; report the previous summary so the
		// caller has something to surface.
		return in.LastRun()
	}
	defer in.mu.Unlock()

	run := IndexRun{StartedAt: time.Now()}
	defer func() {
		run.Duration = time.Since(run.StartedAt).Round(time.Millisecond).String()
		in.lastRunMu.Lock()
		in.lastRun = run
		in.lastRunMu.Unlock()
	}()

	live := make(map[string]struct{})
	host := localHost()

	// Pull tombstones once at the start; user-deleted sessions get skipped
	// entirely so IndexAll can't resurrect them from disk.
	tombs, _ := in.idx.TombstonedIDs()

	for _, p := range providers {
		if ctx.Err() != nil {
			run.LastError = ctx.Err().Error()
			run.Errors++
			return run
		}
		sessions, err := p.ScanSessions(ctx)
		if err != nil {
			run.Errors++
			run.LastError = p.Name() + ": " + err.Error()
			continue
		}
		for _, s := range sessions {
			if _, gone := tombs[s.ID]; gone {
				continue // user deleted this — don't re-index it
			}
			live[s.ID] = struct{}{}
			doc, err := BuildSessionDoc(SessionContent{
				ID:          s.ID,
				Provider:    s.Provider,
				ProjectName: s.ProjectName,
				ProjectPath: s.ProjectPath,
				Model:       s.Model,
				GitBranch:   s.GitBranch,
				Modified:    s.Modified,
				Summary:     s.Summary,
				FirstPrompt: s.FirstPrompt,
				DataPath:    s.DataPath,
			})
			if err != nil {
				run.Errors++
				run.LastError = s.ID + ": " + err.Error()
				continue
			}
			doc.Host = host
			indexed, err := in.idx.PutSession(doc)
			if err != nil {
				run.Errors++
				run.LastError = s.ID + ": " + err.Error()
				continue
			}
			if indexed {
				run.Indexed++
			} else {
				run.Skipped++
			}
		}
	}

	// Orphan cleanup: anything in the index that's no longer reported by
	// a provider AND was originated on THIS host gets dropped. Sessions
	// imported from other machines have host != localHost and are kept,
	// since this machine can't see their underlying transcripts.
	rows, err := in.idx.SessionsByHost(host)
	if err == nil {
		for _, id := range rows {
			if _, alive := live[id]; alive {
				continue
			}
			if err := in.idx.Remove(id); err == nil {
				run.Removed++
			}
		}
	}
	return run
}
