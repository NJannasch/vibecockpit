package board

func DemoBoards() []*Board {
	return []*Board{
		{
			Name:    "web-dashboard",
			Project: "~/Projects/web-dashboard",
			Tasks: []Task{
				{
					ID: "auth-flow", Title: "Add OAuth2 PKCE flow for mobile clients",
					Status: "done", Priority: "high",
					Description: "Implement OAuth2 PKCE flow using the existing auth middleware.\nMust support both iOS and Android redirect URIs.",
					Acceptance:  []string{"PKCE challenge/verifier generated per request", "Token refresh works without re-auth", "Tests cover happy path and expired token"},
					Tool: "claude", Model: "claude-opus-4-6",
					ClaimedBy: "claude-code", Sessions: []string{"demo-c1", "demo-c4"},
					Cost: 60.81, Completed: "2026-04-30T14:30:00Z",
					CreatedBy: "human", CreatedAt: "2026-04-29T09:00:00Z", UpdatedAt: "2026-04-30T14:30:00Z",
					History: []HistoryEntry{
						{Timestamp: "2026-04-29T09:00:00Z", Action: "status", By: "human", Detail: "backlog → claimed"},
						{Timestamp: "2026-04-29T10:15:00Z", Action: "status", By: "mcp-agent", Detail: "claimed → in-progress"},
						{Timestamp: "2026-04-29T10:15:00Z", Action: "session-linked", By: "mcp-agent", Detail: "demo-c1"},
						{Timestamp: "2026-04-30T12:00:00Z", Action: "session-linked", By: "mcp-agent", Detail: "demo-c4"},
						{Timestamp: "2026-04-30T14:30:00Z", Action: "status", By: "mcp-agent", Detail: "in-progress → done"},
					},
					Summary: "Implemented PKCE with challenge/verifier, token refresh, and full test coverage. PR #42 merged.",
				},
				{
					ID: "fix-webhook", Title: "Fix Stripe webhook retry logic",
					Status: "in-progress", Priority: "high",
					Description: "Idempotency keys missing on webhook retries. Duplicate charges occurring.",
					Tool: "claude", Model: "claude-sonnet-4-6",
					ClaimedBy: "claude-code", Sessions: []string{"demo-c2"},
					Cost: 3.77, Started: "2026-05-01T08:00:00Z",
					CreatedBy: "human", CreatedAt: "2026-04-30T16:00:00Z", UpdatedAt: "2026-05-01T10:00:00Z",
					History: []HistoryEntry{
						{Timestamp: "2026-04-30T16:00:00Z", Action: "status", By: "human", Detail: "backlog → claimed"},
						{Timestamp: "2026-05-01T08:00:00Z", Action: "status", By: "mcp-agent", Detail: "claimed → in-progress"},
						{Timestamp: "2026-05-01T08:00:00Z", Action: "session-linked", By: "mcp-agent", Detail: "demo-c2"},
					},
				},
				{
					ID: "add-metrics", Title: "Add Prometheus metrics endpoint",
					Status: "review", Priority: "medium",
					Description: "Expose /metrics with request count, latency histogram, and active connections gauge.",
					Tool: "codex", Model: "openai/gpt-5.4",
					Sessions: []string{"demo-x1"},
					Cost: 6.74, Started: "2026-04-29T14:00:00Z",
					CreatedBy: "mcp-agent", CreatedAt: "2026-04-28T11:00:00Z", UpdatedAt: "2026-05-01T06:00:00Z",
					History: []HistoryEntry{
						{Timestamp: "2026-04-29T14:00:00Z", Action: "status", By: "mcp-agent", Detail: "backlog → in-progress"},
						{Timestamp: "2026-04-29T14:00:00Z", Action: "session-linked", By: "mcp-agent", Detail: "demo-x1"},
						{Timestamp: "2026-05-01T06:00:00Z", Action: "status", By: "mcp-agent", Detail: "in-progress → review"},
					},
					Summary: "Added /metrics endpoint with all three metric types. Grafana dashboard template included.",
				},
				{
					ID: "refactor-db", Title: "Refactor database connection pooling",
					Status: "backlog", Priority: "medium",
					Description: "Current pool settings cause connection exhaustion under load. Switch to pgxpool with configurable limits.",
					Tool: "claude", Model: "claude-opus-4-6",
					CreatedBy: "human", CreatedAt: "2026-05-01T09:00:00Z", UpdatedAt: "2026-05-01T09:00:00Z",
				},
				{
					ID: "dark-mode", Title: "Fix dark mode contrast issues",
					Status: "backlog", Priority: "low",
					Description: "Several text elements have poor contrast in dark mode. Audit all pages and fix.",
					CreatedBy: "mcp-agent", CreatedAt: "2026-04-30T20:00:00Z", UpdatedAt: "2026-04-30T20:00:00Z",
				},
				{
					ID: "update-deps", Title: "Update npm dependencies",
					Status: "done", Priority: "low",
					Tool: "codex", Model: "openai/gpt-5.4",
					Sessions: []string{"demo-x2"},
					Cost: 4.23, Completed: "2026-04-28T18:00:00Z",
					CreatedBy: "human", CreatedAt: "2026-04-28T15:00:00Z", UpdatedAt: "2026-04-28T18:00:00Z",
					History: []HistoryEntry{
						{Timestamp: "2026-04-28T18:00:00Z", Action: "status", By: "mcp-agent", Detail: "in-progress → done"},
						{Timestamp: "2026-04-28T16:00:00Z", Action: "session-linked", By: "mcp-agent", Detail: "demo-x2"},
					},
					Summary: "Updated all deps, fixed 2 breaking changes, all tests pass.",
				},
			},
		},
	}
}
