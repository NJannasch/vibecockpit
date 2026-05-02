package web

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"vibecockpit/internal/costs"
	"vibecockpit/internal/provider"
)

type scanCache struct {
	mu          sync.Mutex
	providers   []provider.Provider
	sessions    []provider.Session
	lastScan    time.Time
	dirModTimes map[string]time.Time
	cachePath   string
}

type cacheFile struct {
	Sessions    []provider.Session `json:"sessions"`
	DirModTimes map[string]string  `json:"dirModTimes"`
	CachedAt    string             `json:"cachedAt"`
}

func newScanCache(providers []provider.Provider) *scanCache {
	home, _ := os.UserHomeDir()
	return &scanCache{
		providers:   providers,
		dirModTimes: make(map[string]time.Time),
		cachePath:   filepath.Join(home, ".config", "vibecockpit", "cache", "sessions.json"),
	}
}

func (sc *scanCache) getSessions(forceRefresh bool) []provider.Session {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if !forceRefresh && sc.sessions != nil && time.Since(sc.lastScan) < 10*time.Second {
		return sc.sessions
	}

	if !forceRefresh && sc.sessions == nil {
		sc.loadFromDisk()
	}

	changed := forceRefresh || sc.sessions == nil
	if !changed {
		for _, p := range sc.providers {
			dir := providerDir(p.Name())
			if dir == "" {
				continue
			}
			info, err := os.Stat(dir)
			if err != nil {
				continue
			}
			prev, ok := sc.dirModTimes[dir]
			if !ok || info.ModTime().After(prev) {
				changed = true
				break
			}
		}
	}

	if !changed {
		sc.lastScan = time.Now()
		return sc.sessions
	}

	var all []provider.Session
	for _, p := range sc.providers {
		sessions, err := p.ScanSessions(context.Background())
		if err != nil {
			continue
		}
		for i := range sessions {
			sessions[i].EstCostUSD = costs.EstimateCost(sessions[i].Model, sessions[i].Tokens)
		}
		all = append(all, sessions...)

		dir := providerDir(p.Name())
		if dir != "" {
			if info, err := os.Stat(dir); err == nil {
				sc.dirModTimes[dir] = info.ModTime()
			}
		}
	}

	sc.sessions = all
	sc.lastScan = time.Now()
	sc.saveToDisk()
	return sc.sessions
}

func (sc *scanCache) loadFromDisk() {
	data, err := os.ReadFile(sc.cachePath)
	if err != nil {
		return
	}
	var cf cacheFile
	if json.Unmarshal(data, &cf) != nil {
		return
	}
	cachedAt, err := time.Parse(time.RFC3339, cf.CachedAt)
	if err != nil || time.Since(cachedAt) > 24*time.Hour {
		return
	}
	sc.sessions = cf.Sessions
	sc.dirModTimes = make(map[string]time.Time)
	for k, v := range cf.DirModTimes {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			sc.dirModTimes[k] = t
		}
	}
	sc.lastScan = cachedAt
}

func (sc *scanCache) saveToDisk() {
	modTimes := make(map[string]string)
	for k, v := range sc.dirModTimes {
		modTimes[k] = v.Format(time.RFC3339)
	}
	cf := cacheFile{
		Sessions:    sc.sessions,
		DirModTimes: modTimes,
		CachedAt:    time.Now().Format(time.RFC3339),
	}
	data, err := json.Marshal(cf)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(sc.cachePath), 0755)
	_ = os.WriteFile(sc.cachePath, data, 0644)
}

func providerDir(name string) string {
	home, _ := os.UserHomeDir()
	switch name {
	case "claude":
		return filepath.Join(home, ".claude", "projects")
	case "codex":
		return filepath.Join(home, ".codex")
	case "copilot":
		return filepath.Join(home, ".config", "github-copilot")
	case "gemini":
		return filepath.Join(home, ".gemini")
	case "opencode":
		return filepath.Join(home, ".opencode")
	case "cursor":
		return filepath.Join(home, ".cursor")
	default:
		return ""
	}
}
