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

## Adding a provider

1. Create `internal/provider/yourprovider/yourprovider.go`
2. Implement `provider.Provider` interface
3. Register in `main.go` → `buildRegistry()`

## Conventions

- No unused variables, imports, or parameters — linters enforce this
- Handle errors or explicitly ignore with `_ =` (never silently drop)
- Frontend: `npx eslint .` must pass with zero errors (warnings are OK)
- Tests go next to the code: `foo.go` → `foo_test.go`
