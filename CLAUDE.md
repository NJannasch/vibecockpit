# VibeCockpit

Go 1.25 backend + Svelte 5 frontend, single binary with embedded web assets.

## Build & Run

```bash
make build          # frontend + Go binary → ./vibecockpit
make run            # build + launch
./build.sh local    # alternative: build via script
```

## Development

```bash
./vibecockpit --web --port 3456 &   # API backend
cd frontend && npm run dev           # Svelte with hot-reload (proxies /api to :3456)
```

## Quality

```bash
make check          # runs lint + test (the full gate)
make lint           # golangci-lint + eslint
make lint-go        # Go only
make lint-frontend  # frontend only
make test           # go test -race ./...
```

CI runs the same checks — if `make check` passes locally, CI will pass.
Always run `make check` locally before pushing.

## Architecture

```
internal/
├── board/       # YAML kanban boards, task CRUD, history, discovery
├── runner/      # Agent spawning, ralph loop, worktree isolation, tracker
├── mcp/         # MCP server (14+ tools: sessions, boards, costs, inventory, search_memory)
├── memory/      # FTS5 cross-tool transcript index (search_memory + /api/memory/search)
├── web/         # HTTP server, REST API, scan cache, SPA handler
├── provider/    # Session scanners (claude, claude-desktop, codex, copilot, gemini, opencode, cursor, antigravity)
├── costs/       # Token pricing, cost estimation, aggregation
├── inventory/   # Tool/extension/MCP/config scanning
├── audit/       # MCP tool call audit log
├── config/      # YAML config, terminal detection
├── launcher/    # Terminal-aware session launcher
├── scanner/     # Secret scanning (gitleaks rules)
├── stats/       # Adoption timeline, usage statistics
└── install/     # Binary installer, desktop entry, autostart
frontend/src/
├── App.svelte          # Main app, routing, sidebar, page bars
├── components/
│   ├── Dashboard.svelte    # Metric cards, active sessions, tool chips
│   ├── BoardView.svelte    # Kanban boards, task modal, drag-and-drop
│   ├── AgentMonitor.svelte # Agent runs, log viewer, diff/merge
│   ├── CostsDashboard.svelte
│   ├── ToolInventory.svelte
│   └── Settings.svelte
└── lib/
    ├── api.js    # REST API client functions
    ├── stores.js # Svelte stores
    └── utils.js  # Shared utilities
```

## Key subsystems

### Board system (`internal/board/`)
- YAML-based kanban boards at `~/.config/vibecockpit/boards/` or `.vibecockpit/board.yaml` per project
- Tasks with status, priority, tool, model, acceptance criteria, history, cost tracking
- CLI: `vibecockpit board {list,show,create,add,move}`

### Agent spawning (`internal/runner/`)
- `vibecockpit run <task-id>`: spawn headless agents on board tasks
- Git worktree isolation for every run
- Ralph loop: auto-retry on failed `run:` acceptance criteria
- Tool-native configs: MCP + permissions per tool (Claude/Codex/Gemini/OpenCode)
- Agent tracker with PID, elapsed time, log tailing, kill support

### MCP server (`internal/mcp/`)
- 13+ tools: sessions, boards, costs, stats, inventory, rescan
- Board tools: list_boards, list_tasks, get_task, update_task, create_task
- Inventory filtering by type and query
- Run via `vibecockpit --mcp`

### Scan cache (`internal/web/scancache.go`)
- Directory mtime-based invalidation
- Disk persistence at `~/.config/vibecockpit/cache/sessions.json`
- All handlers use the cache (sessions, costs, inventory, stats, boards)

## Adding a provider

1. Create `internal/provider/yourprovider/yourprovider.go`
2. Implement `provider.Provider` interface
3. Register in `main.go` → `buildRegistry()`

## Conventions

- No unused variables, imports, or parameters — linters enforce this
- Handle errors or explicitly ignore with `_ =` (never silently drop)
- Frontend: `npx eslint .` must pass with zero errors (3 intentional `@html` warnings OK)
- Tests go next to the code: `foo.go` → `foo_test.go`
- Every `{#each}` block needs a key expression
- `{@const}` must be inside `{#if}`, `{#each}`, or `{#snippet}` — use `$derived` at script level instead
- Cost values labeled as "est." or "~" (estimated API cost, not actual charges)
- Board tasks track cost via snapshot deltas (costAtStart/costAtEnd)
