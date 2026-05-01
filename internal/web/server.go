package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"vibecockpit/internal/audit"
	"vibecockpit/internal/board"
	"vibecockpit/internal/config"
	"vibecockpit/internal/costs"
	"vibecockpit/internal/inventory"
	"vibecockpit/internal/launcher"
	"vibecockpit/internal/provider"
	"vibecockpit/internal/scanner"
	"vibecockpit/internal/stats"
)

//go:embed all:static
var staticFiles embed.FS

type server struct {
	cfg           *config.Config
	providers     []provider.Provider
	version       string
	cachedResult  []apiSession
	cachedAt      time.Time
	cacheTTL      time.Duration
	secretScanner *scanner.Scanner

	inventoryCache    *inventory.Inventory
	inventoryCachedAt time.Time
	inventoryTTL      time.Duration
	demoMode          bool

	auditLog *audit.Logger

	versionMu      sync.Mutex
	latestVersion  string
	latestFetched  time.Time
}

type apiSession struct {
	ID           string              `json:"id"`
	Provider     string              `json:"provider"`
	ProjectName  string              `json:"projectName"`
	ProjectPath  string              `json:"projectPath"`
	Summary      string              `json:"summary"`
	FirstPrompt  string              `json:"firstPrompt"`
	Model        string              `json:"model"`
	GitBranch    string              `json:"gitBranch"`
	MessageCount int                 `json:"messageCount"`
	Tokens       provider.TokenUsage `json:"tokens,omitempty"`
	EstCostUSD   float64             `json:"estCostUsd,omitempty"`
	Modified     string              `json:"modified,omitempty"`
	Created      string              `json:"created,omitempty"`
	IsActive     bool                `json:"isActive"`
	ActivePID    int    `json:"activePID,omitempty"`
}

type apiConfig struct {
	Terminal           string   `json:"terminal"`
	NewProjectDir      string   `json:"newProjectDir"`
	CustomTermCmd      string   `json:"customTerminalCmd,omitempty"`
	Theme              string   `json:"theme"`
	SortBy             string   `json:"sortBy"`
	GroupBy            string   `json:"groupBy"`
	AvailableTerminals []string           `json:"availableTerminals,omitempty"`
	Models             []string           `json:"models,omitempty"`
	ProviderPaths      map[string]string  `json:"providerPaths,omitempty"`
	RemoteSources      []map[string]any   `json:"remoteSources,omitempty"`
	EnableScanner      bool               `json:"enableScanner"`
	EnableMCP          bool               `json:"enableMcp"`
	ExtraPath          []string           `json:"extraPath,omitempty"`
	ScanSkipRules      []string           `json:"scanSkipRules,omitempty"`
	ScanExtraHints     []string           `json:"scanExtraHints,omitempty"`
	DisabledProviders  []string           `json:"disabledProviders"`
	AllProviders       []providerInfo     `json:"allProviders,omitempty"`
}

type providerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func Start(cfg *config.Config, providers []provider.Provider, port int, version string) error {
	isDemo := len(providers) == 1 && providers[0].Name() == "demo"
	s := &server{
		cfg: cfg, providers: providers, version: version,
		cacheTTL:     10 * time.Second,
		inventoryTTL: 60 * time.Second,
		demoMode:     isDemo,
		auditLog:     audit.NewLogger(),
		secretScanner: func() *scanner.Scanner {
			if cfg.EnableScanner {
				return scanner.New(providers, scanner.Config{
					SkipRules:  cfg.ScanSkipRules,
					ExtraHints: cfg.ScanExtraHints,
				})
			}
			return nil
		}(),
	}

	if cfg.Terminal == "default" || cfg.Terminal == "" {
		cfg.Terminal = detectTerminal()
		_ = cfg.Save()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("POST /api/launch", s.handleLaunch)
	mux.HandleFunc("DELETE /api/sessions", s.handleDelete)
	mux.HandleFunc("POST /api/new", s.handleNew)
	mux.HandleFunc("POST /api/test-ssh", s.handleTestSSH)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	mux.HandleFunc("GET /api/costs", s.handleCosts)
	mux.HandleFunc("GET /api/inventory", s.handleInventory)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/inventory/file", s.handleInventoryFile)
	mux.HandleFunc("GET /api/version", s.handleVersion)
	mux.HandleFunc("GET /api/mcp-audit", s.handleMCPAudit)
	mux.HandleFunc("GET /api/boards", s.handleGetBoards)
	mux.HandleFunc("GET /api/boards/{name}", s.handleGetBoard)
	mux.HandleFunc("POST /api/boards", s.handleCreateBoard)
	mux.HandleFunc("POST /api/boards/{name}/tasks", s.handleAddTask)
	mux.HandleFunc("PUT /api/boards/{name}/tasks/{id}", s.handleUpdateTask)
	if cfg.EnableScanner {
		mux.HandleFunc("POST /api/scan-secrets", s.handleStartScan)
		mux.HandleFunc("GET /api/scan-secrets", s.handleScanStatus)
	}

	sub, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		if isAddrInUse(err) {
			killed, killErr := killStaleVibecockpit(port)
			if killErr != nil {
				return fmt.Errorf("port %d in use and could not free it: %w", port, killErr)
			}
			if killed {
				fmt.Printf("Stopped previous vibecockpit instance on port %d\n", port)
			}
			listener, err = net.Listen("tcp", addr)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	fmt.Printf("VibeCockpit web UI: http://%s\n", addr)
	fmt.Println("Press Ctrl+C to stop")
	openBrowser(fmt.Sprintf("http://%s", addr))

	return http.Serve(listener, mux)
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	// errors.Is(err, syscall.EADDRINUSE) is the principled check, but the
	// net package wraps the error in ways that can break the chain across
	// versions. The OS-level message is stable, so we accept either path.
	if errno, ok := errnoOf(err); ok && errno == syscall.EADDRINUSE {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "address in use") ||
		strings.Contains(msg, "Only one usage of each socket address")
}

func errnoOf(err error) (syscall.Errno, bool) {
	for ; err != nil; err = unwrap(err) {
		if e, ok := err.(syscall.Errno); ok {
			return e, true
		}
	}
	return 0, false
}

func unwrap(err error) error {
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return nil
}

// killStaleVibecockpit looks up whoever is bound to port and, if it's
// another vibecockpit process, signals it to shut down so a fresh
// instance can take over. Refuses to kill anything else; returns
// (killed, err). On platforms without lsof it's a no-op.
func killStaleVibecockpit(port int) (bool, error) {
	if _, err := exec.LookPath("lsof"); err != nil {
		return false, nil
	}
	out, err := exec.Command("lsof", "-t", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN").Output()
	if err != nil {
		// lsof exits non-zero when no match; that's fine.
		return false, nil
	}
	pids := strings.Fields(strings.TrimSpace(string(out)))
	if len(pids) == 0 {
		return false, nil
	}

	self := os.Getpid()
	var targets []int
	for _, p := range pids {
		pid, err := strconv.Atoi(p)
		if err != nil || pid == self {
			continue
		}
		comm, err := exec.Command("ps", "-p", p, "-o", "comm=").Output()
		if err != nil {
			continue
		}
		base := filepath.Base(strings.TrimSpace(string(comm)))
		if base != "vibecockpit" {
			return false, fmt.Errorf("port %d held by %s (pid %d); refusing to kill", port, base, pid)
		}
		targets = append(targets, pid)
	}
	if len(targets) == 0 {
		return false, nil
	}

	for _, pid := range targets {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(syscall.SIGTERM)
		}
	}
	// Wait up to 2s for the port to free; SIGKILL anything still holding it.
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for i := 0; i < 20; i++ {
		l, err := net.Listen("tcp", addr)
		if err == nil {
			_ = l.Close()
			return true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, pid := range targets {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(syscall.SIGKILL)
		}
	}
	time.Sleep(300 * time.Millisecond)
	return true, nil
}

func (s *server) handleSessions(w http.ResponseWriter, r *http.Request) {
	forceRefresh := r.URL.Query().Get("refresh") == "true"

	if !forceRefresh && s.cachedResult != nil && time.Since(s.cachedAt) < s.cacheTTL {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.cachedResult)
		return
	}

	var all []provider.Session
	for _, p := range s.providers {
		sessions, err := p.ScanSessions(context.Background())
		if err != nil {
			continue
		}
		all = append(all, sessions...)
	}

	out := make([]apiSession, 0, len(all))
	for _, sess := range all {
		estCost := costs.EstimateCost(sess.Model, sess.Tokens)
		as := apiSession{
			ID:           sess.ID,
			Provider:     sess.Provider,
			ProjectName:  sess.ProjectName,
			ProjectPath:  sess.ProjectPath,
			Summary:      sess.Summary,
			FirstPrompt:  sess.FirstPrompt,
			Model:        sess.Model,
			GitBranch:    sess.GitBranch,
			MessageCount: sess.MessageCount,
			Tokens:       sess.Tokens,
			EstCostUSD:   estCost,
			IsActive:     sess.IsActive,
			ActivePID:    sess.ActivePID,
		}
		if !sess.Modified.IsZero() {
			as.Modified = sess.Modified.Format("2006-01-02T15:04:05Z")
		}
		if !sess.Created.IsZero() {
			as.Created = sess.Created.Format("2006-01-02T15:04:05Z")
		}
		out = append(out, as)
	}

	s.cachedResult = out
	s.cachedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID     string `json:"sessionId"`
		Provider      string `json:"provider"`
		ModelOverride string `json:"model,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}

	prov := s.findProvider(req.Provider)
	if prov == nil {
		jsonError(w, "unknown provider: "+req.Provider, 400)
		return
	}

	sess, err := s.findSession(prov, req.SessionID)
	if err != nil {
		jsonError(w, "session not found", 404)
		return
	}

	if req.ModelOverride != "" {
		sess.Model = req.ModelOverride
	}

	if err := launcher.Launch(s.cfg, prov, *sess); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"sessionId"`
		Provider  string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}

	prov := s.findProvider(req.Provider)
	if prov == nil {
		jsonError(w, "unknown provider: "+req.Provider, 400)
		return
	}

	sess, err := s.findSession(prov, req.SessionID)
	if err != nil {
		jsonError(w, "session not found", 404)
		return
	}

	if sess.IsActive {
		jsonError(w, "cannot delete an active session — stop it first", 400)
		return
	}

	if err := prov.DeleteSession(req.SessionID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *server) handleNew(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dir   string `json:"dir"`
		Tool  string `json:"tool"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Dir == "" {
		jsonError(w, "directory path required", 400)
		return
	}

	if err := os.MkdirAll(req.Dir, 0755); err != nil {
		jsonError(w, "could not create directory: "+err.Error(), 500)
		return
	}

	var prov provider.Provider
	if req.Tool != "" {
		for _, p := range s.providers {
			if p.Name() == req.Tool {
				prov = p
				break
			}
		}
	}
	if prov == nil && len(s.providers) > 0 {
		prov = s.providers[0]
	}
	if prov == nil {
		jsonError(w, "no providers available", 500)
		return
	}

	if err := launcher.LaunchNew(s.cfg, prov, req.Dir); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *server) handleTestSSH(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host   string `json:"host"`
		User   string `json:"user"`
		Port   int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}
	if req.Host == "" || req.User == "" {
		jsonError(w, "host and user required", 400)
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}

	args := []string{
		fmt.Sprintf("%s@%s", req.User, req.Host),
		"-p", fmt.Sprintf("%d", req.Port),
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"echo vibecockpit-ok",
	}

	out, err := exec.CommandContext(r.Context(), "ssh", args...).CombinedOutput()
	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": strings.TrimSpace(string(out)),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": strings.TrimSpace(string(out)),
	})
}

func (s *server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	allProviders := []providerInfo{
		{ID: "claude", Name: "Claude Code"},
		{ID: "claude-desktop", Name: "Claude Desktop"},
		{ID: "opencode", Name: "OpenCode"},
		{ID: "codex", Name: "Codex CLI"},
		{ID: "copilot", Name: "Copilot CLI"},
		{ID: "gemini", Name: "Gemini CLI"},
		{ID: "cursor", Name: "Cursor Agent"},
		{ID: "antigravity", Name: "Antigravity"},
	}
	disabled := s.cfg.DisabledProviders
	if disabled == nil {
		disabled = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiConfig{
		Terminal:           s.cfg.Terminal,
		NewProjectDir:      s.cfg.NewProjectDir,
		CustomTermCmd:      s.cfg.CustomTermCmd,
		Theme:              s.cfg.Theme,
		SortBy:             s.cfg.SortBy,
		GroupBy:            s.cfg.GroupBy,
		AvailableTerminals: config.AvailableTerminals(),
		Models:             config.Models,
		ProviderPaths:      s.cfg.ProviderPaths,
		EnableScanner:      s.cfg.EnableScanner,
		EnableMCP:          s.cfg.EnableMCP,
		RemoteSources:      s.cfg.RemoteSources,
		ExtraPath:          s.cfg.ExtraPath,
		ScanSkipRules:      s.cfg.ScanSkipRules,
		ScanExtraHints:     s.cfg.ScanExtraHints,
		DisabledProviders:  disabled,
		AllProviders:       allProviders,
	})
}

func (s *server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var req apiConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid config", 400)
		return
	}

	s.cfg.Terminal = req.Terminal
	s.cfg.NewProjectDir = req.NewProjectDir
	s.cfg.CustomTermCmd = req.CustomTermCmd
	if req.Theme != "" {
		s.cfg.Theme = req.Theme
	}
	if req.SortBy != "" {
		s.cfg.SortBy = req.SortBy
	}
	if req.GroupBy != "" {
		s.cfg.GroupBy = req.GroupBy
	}
	if req.ProviderPaths != nil {
		s.cfg.ProviderPaths = req.ProviderPaths
	}
	if req.RemoteSources != nil {
		s.cfg.RemoteSources = req.RemoteSources
	}
	if req.ExtraPath != nil {
		s.cfg.ExtraPath = req.ExtraPath
	}
	if req.ScanSkipRules != nil {
		s.cfg.ScanSkipRules = req.ScanSkipRules
	}
	if req.ScanExtraHints != nil {
		s.cfg.ScanExtraHints = req.ScanExtraHints
	}
	if req.DisabledProviders != nil {
		s.cfg.DisabledProviders = req.DisabledProviders
	}
	s.cfg.EnableMCP = req.EnableMCP
	s.cfg.EnableScanner = req.EnableScanner
	if err := s.cfg.Save(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *server) findProvider(name string) provider.Provider {
	for _, p := range s.providers {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

func (s *server) findSession(prov provider.Provider, id string) (*provider.Session, error) {
	sessions, err := prov.ScanSessions(context.Background())
	if err != nil {
		return nil, err
	}
	for _, sess := range sessions {
		if sess.ID == id {
			return &sess, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return
	}
	_ = cmd.Start()
}

func detectTerminal() string {
	if runtime.GOOS == "darwin" {
		for _, t := range []string{"iterm2", "kitty", "wezterm", "alacritty", "ghostty"} {
			if _, err := exec.LookPath(t); err == nil {
				return t
			}
		}
		return "terminal.app"
	}
	for _, t := range []string{"kitty", "wezterm", "alacritty", "ghostty", "gnome-terminal", "konsole", "xterm"} {
		if _, err := exec.LookPath(t); err == nil {
			return t
		}
	}
	return "xterm"
}

const (
	githubLatestURL    = "https://api.github.com/repos/NJannasch/vibecockpit/releases/latest"
	latestVersionTTL   = time.Hour
	latestFetchTimeout = 3 * time.Second
)

type versionResponse struct {
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
}

func (s *server) handleCosts(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			days = n
		}
	}
	since := time.Now().AddDate(0, 0, -days)

	var all []provider.Session
	for _, p := range s.providers {
		sessions, err := p.ScanSessions(context.Background())
		if err != nil {
			continue
		}
		for i := range sessions {
			sessions[i].EstCostUSD = costs.EstimateCost(sessions[i].Model, sessions[i].Tokens)
		}
		all = append(all, sessions...)
	}

	summary := costs.Aggregate(all, since)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (s *server) handleInventory(w http.ResponseWriter, r *http.Request) {
	forceRefresh := r.URL.Query().Get("refresh") == "true"

	if !forceRefresh && s.inventoryCache != nil && time.Since(s.inventoryCachedAt) < s.inventoryTTL {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.inventoryCache)
		return
	}

	var inv *inventory.Inventory
	if s.demoMode {
		inv = inventory.Demo()
	} else {
		var all []provider.Session
		for _, p := range s.providers {
			sessions, err := p.ScanSessions(context.Background())
			if err != nil {
				continue
			}
			all = append(all, sessions...)
		}
		inv = inventory.Scan(all, s.cfg.NewProjectDir)
	}

	s.inventoryCache = inv
	s.inventoryCachedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

func (s *server) handleInventoryFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	path = filepath.Clean(path)

	allowed := false
	if inv := s.inventoryCache; inv != nil {
		for _, f := range inv.InstructionFiles {
			if filepath.Clean(f.Path) == path {
				allowed = true
				break
			}
		}
		if !allowed {
			for _, m := range inv.MCPServers {
				if filepath.Clean(m.SourcePath) == path {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			for _, sk := range inv.Skills {
				if filepath.Clean(sk.Path) == path {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			for _, mem := range inv.Memories {
				if filepath.Clean(mem.Path) == path {
					allowed = true
					break
				}
			}
		}
	}

	if !allowed {
		http.Error(w, "file not in inventory", http.StatusForbidden)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "could not read file", http.StatusNotFound)
		return
	}

	const maxSize = 512 * 1024
	if len(data) > maxSize {
		data = data[:maxSize]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"path":    path,
		"content": string(data),
	})
}

func (s *server) handleStats(w http.ResponseWriter, r *http.Request) {
	var all []provider.Session
	for _, p := range s.providers {
		sessions, err := p.ScanSessions(context.Background())
		if err != nil {
			continue
		}
		all = append(all, sessions...)
	}

	var inv *inventory.Inventory
	if s.inventoryCache != nil && time.Since(s.inventoryCachedAt) < s.inventoryTTL {
		inv = s.inventoryCache
	} else if s.demoMode {
		inv = inventory.Demo()
	} else {
		inv = inventory.Scan(all, s.cfg.NewProjectDir)
		s.inventoryCache = inv
		s.inventoryCachedAt = time.Now()
	}

	result := stats.Compute(all, inv)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *server) handleStartScan(w http.ResponseWriter, r *http.Request) {
	s.secretScanner.Start()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *server) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.secretScanner.GetStatus())
}

func (s *server) handleVersion(w http.ResponseWriter, r *http.Request) {
	resp := versionResponse{Current: s.version}
	if latest := s.getLatestVersion(); latest != "" {
		resp.Latest = latest
		resp.UpdateAvailable = compareSemver(s.version, latest) < 0
		resp.ReleaseURL = "https://github.com/NJannasch/vibecockpit/releases/tag/" + latest
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *server) handleMCPAudit(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	entries, err := s.auditLog.ReadLog(limit)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if entries == nil {
		entries = []audit.Entry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (s *server) handleGetBoards(w http.ResponseWriter, _ *http.Request) {
	boards, err := board.Discover(s.cfg.NewProjectDir)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(boards)
}

func (s *server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	boards, _ := board.Discover(s.cfg.NewProjectDir)
	b := board.FindBoard(boards, name)
	if b == nil {
		jsonError(w, "board not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(b)
}

func (s *server) handleCreateBoard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Project string `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		jsonError(w, "name required", 400)
		return
	}
	if req.Project == "" {
		req.Project = "."
	}
	b, err := board.CreateBoard(req.Name, req.Project)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(b)
}

func (s *server) handleAddTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Title       string `json:"title"`
		Priority    string `json:"priority"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
		jsonError(w, "title required", 400)
		return
	}
	boards, _ := board.Discover(s.cfg.NewProjectDir)
	b := board.FindBoard(boards, name)
	if b == nil {
		jsonError(w, "board not found", 404)
		return
	}
	task := b.AddTask(req.Title, req.Priority, req.Description)
	if err := b.Save(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (s *server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	boardName := r.PathValue("name")
	taskID := r.PathValue("id")
	var req struct {
		Status   string `json:"status"`
		Priority string `json:"priority"`
		Summary  string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}
	boards, _ := board.Discover(s.cfg.NewProjectDir)
	b := board.FindBoard(boards, boardName)
	if b == nil {
		jsonError(w, "board not found", 404)
		return
	}
	t, _ := b.FindTask(taskID)
	if t == nil {
		jsonError(w, "task not found", 404)
		return
	}
	if req.Status != "" {
		if err := b.MoveTask(taskID, req.Status); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
	}
	if req.Priority != "" {
		t.Priority = req.Priority
	}
	if req.Summary != "" {
		t.Summary = req.Summary
	}
	if err := b.Save(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

// getLatestVersion returns the cached latest release tag, refreshing it
// from the GitHub API at most once per latestVersionTTL. A failed fetch
// is silent — the caller just gets the previously-cached value (or "").
func (s *server) getLatestVersion() string {
	s.versionMu.Lock()
	if s.latestVersion != "" && time.Since(s.latestFetched) < latestVersionTTL {
		v := s.latestVersion
		s.versionMu.Unlock()
		return v
	}
	s.versionMu.Unlock()

	tag := fetchLatestTag()
	if tag == "" {
		return ""
	}
	s.versionMu.Lock()
	s.latestVersion = tag
	s.latestFetched = time.Now()
	s.versionMu.Unlock()
	return tag
}

func fetchLatestTag() string {
	ctx, cancel := context.WithTimeout(context.Background(), latestFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", githubLatestURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "vibecockpit")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ""
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	return body.TagName
}

// compareSemver returns negative if a < b, zero if equal, positive if a > b.
// Strips a "v" prefix and compares dotted numeric components. Non-numeric
// segments compare as zero. Treats "dev" / unknown versions as smaller than
// any real release, so a dev binary always shows "update available" when a
// release exists.
func compareSemver(a, b string) int {
	if a == "" || a == "dev" {
		if b == "" || b == "dev" {
			return 0
		}
		return -1
	}
	if b == "" || b == "dev" {
		return 1
	}
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(ap) {
			ai, _ = strconv.Atoi(ap[i])
		}
		if i < len(bp) {
			bi, _ = strconv.Atoi(bp[i])
		}
		if ai != bi {
			return ai - bi
		}
	}
	return 0
}
