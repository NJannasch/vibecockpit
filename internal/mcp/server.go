package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"vibecockpit/internal/audit"
	"vibecockpit/internal/costs"
	"vibecockpit/internal/inventory"
	"vibecockpit/internal/provider"
	"vibecockpit/internal/sanitize"
	"vibecockpit/internal/scanner"
	"vibecockpit/internal/search"
	"vibecockpit/internal/stats"
)

type Server struct {
	providers    []provider.Provider
	version      string
	workspaceDir string
	audit        *audit.Logger
}

func NewServer(providers []provider.Provider, version, workspaceDir string) *Server {
	return &Server{
		providers:    providers,
		version:      version,
		workspaceDir: workspaceDir,
		audit:        audit.NewLogger(),
	}
}

// Run starts the MCP server, reading JSON-RPC from stdin and writing to stdout.
func (s *Server) Run() error {
	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeError(writer, nil, -32700, "Parse error")
			continue
		}

		s.handle(writer, &req)
	}
}

func (s *Server) handle(w io.Writer, req *jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		writeResult(w, req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "vibecockpit",
				"version": s.version,
			},
		})

	case "notifications/initialized":
		// no response needed

	case "tools/list":
		writeResult(w, req.ID, map[string]any{
			"tools": s.toolDefinitions(),
		})

	case "tools/call":
		s.handleToolCall(w, req)

	default:
		writeError(w, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func (s *Server) toolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "list_sessions",
			"description": "List all AI coding sessions across all detected tools (Claude Code, Codex, Copilot, Gemini, OpenCode, remote). Returns sanitized session metadata.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{
					"provider": map[string]any{
						"type":        "string",
						"description": "Filter by provider name (e.g. 'claude', 'codex'). Omit for all.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of sessions to return. Default 20.",
					},
				},
			},
		},
		{
			"name":        "search_sessions",
			"description": "Fuzzy search across all coding sessions. Supports structured filters like 'model:opus', 'branch:main', 'active'. Returns sanitized results ranked by relevance.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query. Supports fuzzy text and filters (model:X, branch:X, provider:X, active).",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "get_session_detail",
			"description": "Get detailed information about a specific coding session. Returns sanitized metadata including project path, model, branch, message count, and timestamps.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "The session ID to look up.",
					},
				},
				"required": []string{"session_id"},
			},
		},
		{
			"name":        "scan_secrets",
			"description": "Scan all AI coding session files for leaked secrets (API keys, tokens, passwords, private keys). Uses 200+ gitleaks detection rules. Returns findings with redacted matches — never exposes the actual secret values.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{
					"provider": map[string]any{
						"type":        "string",
						"description": "Scan only sessions from this provider. Omit for all.",
					},
				},
			},
		},
		{
			"name":        "get_costs",
			"description": "Get cost breakdown for AI coding sessions. Returns total spend, per-provider costs, per-project costs, and daily cost history.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"days": map[string]any{
						"type":        "integer",
						"description": "Number of days to look back. Default 30.",
					},
				},
			},
		},
		{
			"name":        "get_stats",
			"description": "Get adoption statistics and timeline. Returns tool usage stats, adoption timeline events, artifact inventory, and summary metrics like first activity date, most active tool/project, and total counts.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "get_inventory",
			"description": "Get tool inventory: installed AI tools, models in use, MCP servers, instruction files (CLAUDE.md, .cursorrules, etc.), skills, memories, IDE extensions, and sensitive files detected.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "rescan",
			"description": "Trigger a fresh scan. Returns updated data for the requested scope. Use after making changes that should be reflected in VibeCockpit.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"scope": map[string]any{
						"type":        "string",
						"description": "What to rescan: 'all' (default), 'sessions', 'inventory', 'stats', or 'costs'.",
						"enum":        []string{"all", "sessions", "inventory", "stats", "costs"},
					},
					"provider": map[string]any{
						"type":        "string",
						"description": "Filter by provider (only applies to sessions/costs scope). Omit for all.",
					},
				},
			},
		},
	}
}

func (s *Server) handleToolCall(w io.Writer, req *jsonRPCRequest) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &call); err != nil {
		writeError(w, req.ID, -32602, "Invalid params")
		return
	}

	var result any
	var resultJSON []byte
	var count int

	switch call.Name {
	case "list_sessions":
		var args struct {
			Provider string `json:"provider"`
			Limit    int    `json:"limit"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			writeError(w, req.ID, -32602, "Invalid arguments: "+err.Error())
			return
		}
		if args.Limit == 0 {
			args.Limit = 20
		}
		result, count = s.listSessions(args.Provider, args.Limit)

	case "search_sessions":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			writeError(w, req.ID, -32602, "Invalid arguments: "+err.Error())
			return
		}
		result, count = s.searchSessions(args.Query)

	case "get_session_detail":
		var args struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			writeError(w, req.ID, -32602, "Invalid arguments: "+err.Error())
			return
		}
		result, count = s.getSessionDetail(args.SessionID)

	case "scan_secrets":
		sc := scanner.New(s.providers)
		sc.Start()
		// Poll until done (max 120s)
		deadline := time.Now().Add(120 * time.Second)
		var status scanner.Status
		for time.Now().Before(deadline) {
			status = sc.GetStatus()
			if status.State == "done" {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		result = status
		count = status.FindingCount

	case "get_costs":
		var args struct {
			Days int `json:"days"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			writeError(w, req.ID, -32602, "Invalid arguments: "+err.Error())
			return
		}
		if args.Days == 0 {
			args.Days = 30
		}
		all := s.allSessions()
		for i := range all {
			all[i].EstCostUSD = costs.EstimateCost(all[i].Model, all[i].Tokens)
		}
		since := time.Now().AddDate(0, 0, -args.Days)
		result = costs.Aggregate(all, since)
		count = len(all)

	case "get_stats":
		all := s.allSessions()
		inv := inventory.Scan(all, s.workspaceDir)
		result = stats.Compute(all, inv)
		count = len(all)

	case "get_inventory":
		all := s.allSessions()
		result = inventory.Scan(all, s.workspaceDir)
		count = 1

	case "rescan":
		var args struct {
			Scope    string `json:"scope"`
			Provider string `json:"provider"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			writeError(w, req.ID, -32602, "Invalid arguments: "+err.Error())
			return
		}
		if args.Scope == "" {
			args.Scope = "all"
		}

		out := map[string]any{"scope": args.Scope}
		all := s.allSessions()

		switch args.Scope {
		case "sessions":
			sessions, _ := s.listSessions(args.Provider, 1000)
			out["sessions"] = sessions
			count = len(all)
		case "inventory":
			out["inventory"] = inventory.Scan(all, s.workspaceDir)
			count = 1
		case "stats":
			inv := inventory.Scan(all, s.workspaceDir)
			out["stats"] = stats.Compute(all, inv)
			count = len(all)
		case "costs":
			for i := range all {
				all[i].EstCostUSD = costs.EstimateCost(all[i].Model, all[i].Tokens)
			}
			since := time.Now().AddDate(0, 0, -30)
			out["costs"] = costs.Aggregate(all, since)
			count = len(all)
		default: // "all"
			sessions, _ := s.listSessions(args.Provider, 1000)
			for i := range all {
				all[i].EstCostUSD = costs.EstimateCost(all[i].Model, all[i].Tokens)
			}
			inv := inventory.Scan(all, s.workspaceDir)
			since := time.Now().AddDate(0, 0, -30)
			out["sessions"] = sessions
			out["inventory"] = inv
			out["stats"] = stats.Compute(all, inv)
			out["costs"] = costs.Aggregate(all, since)
			count = len(all)
		}

		result = out

	default:
		writeError(w, req.ID, -32602, fmt.Sprintf("Unknown tool: %s", call.Name))
		return
	}

	resultJSON, _ = json.Marshal(result)
	s.audit.Log(call.Name, call.Arguments, resultJSON, count)

	writeResult(w, req.ID, map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(resultJSON)},
		},
	})
}

func (s *Server) allSessions() []provider.Session {
	var all []provider.Session
	for _, p := range s.providers {
		sessions, err := p.ScanSessions(context.Background())
		if err != nil {
			continue
		}
		all = append(all, sessions...)
	}
	return all
}

type sessionSummary struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	ProjectName  string `json:"projectName"`
	ProjectPath  string `json:"projectPath,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Model        string `json:"model,omitempty"`
	GitBranch    string `json:"gitBranch,omitempty"`
	MessageCount int    `json:"messageCount"`
	Modified     string `json:"modified,omitempty"`
	IsActive     bool   `json:"isActive,omitempty"`
}

func sanitizeSession(sess provider.Session) sessionSummary {
	path := sess.ProjectPath
	if sanitize.SensitivePath(path) {
		path = "[redacted path]"
	}

	summary := sanitize.Text(sess.Summary)
	if summary == "" {
		summary = sanitize.Text(sess.FirstPrompt)
		if len(summary) > 100 {
			summary = summary[:100] + "..."
		}
	}

	return sessionSummary{
		ID:           sess.ID,
		Provider:     sess.Provider,
		ProjectName:  sess.ProjectName,
		ProjectPath:  path,
		Summary:      summary,
		Model:        sess.Model,
		GitBranch:    sess.GitBranch,
		MessageCount: sess.MessageCount,
		Modified:     sess.Modified.Format("2006-01-02T15:04:05Z"),
		IsActive:     sess.IsActive,
	}
}

func (s *Server) listSessions(providerFilter string, limit int) (any, int) {
	all := s.allSessions()
	var filtered []sessionSummary
	for _, sess := range all {
		if providerFilter != "" && sess.Provider != providerFilter {
			continue
		}
		filtered = append(filtered, sanitizeSession(sess))
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, len(filtered)
}

func (s *Server) searchSessions(query string) (any, int) {
	q := search.ParseQuery(query)
	all := s.allSessions()

	type scored struct {
		session sessionSummary
		score   int
	}
	var results []scored

	for _, sess := range all {
		if q.ActiveOnly && !sess.IsActive {
			continue
		}

		filterOk := true
		for key, val := range q.Filters {
			var field string
			switch key {
			case "model":
				field = sess.Model
			case "branch":
				field = sess.GitBranch
			case "project":
				field = sess.ProjectName
			case "provider":
				field = sess.Provider
			}
			if !strings.Contains(strings.ToLower(field), val) {
				filterOk = false
				break
			}
		}
		if !filterOk {
			continue
		}

		if len(q.FuzzyTerms) == 0 {
			results = append(results, scored{sanitizeSession(sess), 0})
			continue
		}

		summary := sess.Summary
		if summary == "" {
			summary = sess.FirstPrompt
		}

		ok, score := search.FuzzyMatchMulti(q.FuzzyTerms,
			sess.ProjectName, summary, sess.GitBranch, sess.Model)
		if ok {
			results = append(results, scored{sanitizeSession(sess), score})
		}
	}

	// Sort by score desc
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	limit := 20
	if len(results) > limit {
		results = results[:limit]
	}

	out := make([]sessionSummary, len(results))
	for i, r := range results {
		out[i] = r.session
	}
	return out, len(out)
}

func (s *Server) getSessionDetail(sessionID string) (any, int) {
	all := s.allSessions()
	for _, sess := range all {
		if sess.ID == sessionID {
			return sanitizeSession(sess), 1
		}
	}
	return map[string]string{"error": "session not found"}, 0
}

// JSON-RPC types

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func writeResult(w io.Writer, id any, result any) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(w, "%s\n", data)
}

func writeError(w io.Writer, id any, code int, msg string) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": msg},
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(w, "%s\n", data)
}
