package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"vibecockpit/internal/config"
	"vibecockpit/internal/launcher"
	"vibecockpit/internal/provider"
)

//go:embed all:static
var staticFiles embed.FS

type server struct {
	cfg           *config.Config
	providers     []provider.Provider
	cachedResult  []apiSession
	cachedAt      time.Time
	cacheTTL      time.Duration
}

type apiSession struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	ProjectName  string `json:"projectName"`
	ProjectPath  string `json:"projectPath"`
	Summary      string `json:"summary"`
	FirstPrompt  string `json:"firstPrompt"`
	Model        string `json:"model"`
	GitBranch    string `json:"gitBranch"`
	MessageCount int    `json:"messageCount"`
	Modified     string `json:"modified,omitempty"`
	Created      string `json:"created,omitempty"`
	IsActive     bool   `json:"isActive"`
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
}

func Start(cfg *config.Config, providers []provider.Provider, port int) error {
	s := &server{cfg: cfg, providers: providers, cacheTTL: 10 * time.Second}

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

	sub, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("VibeCockpit web UI: http://%s\n", addr)
	fmt.Println("Press Ctrl+C to stop")
	openBrowser(fmt.Sprintf("http://%s", addr))

	return http.ListenAndServe(addr, mux)
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
		RemoteSources:      s.cfg.RemoteSources,
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
	cmd.Start()
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
