BINARY := vibecockpit

.PHONY: build run clean docker dist frontend lint lint-go lint-frontend test check

frontend:
	cd frontend && npm ci && npm run build

build: frontend
	go build -o $(BINARY) .

run: build
	./$(BINARY)

dev:
	@echo "Start Go API:  ./vibecockpit --web --port 3456"
	@echo "Start Svelte:  cd frontend && npm run dev"

# ─── Quality ───────────────────────────────────────────

lint: lint-go lint-frontend

lint-go:
	golangci-lint run ./...

lint-frontend:
	cd frontend && npx eslint .

test:
	go test -race ./...

check: lint test
	@echo "All checks passed."

# ─── Build / Release ──────────────────────────────────

clean:
	rm -rf $(BINARY) dist/ frontend/node_modules internal/web/static

docker:
	docker build -t $(BINARY)-builder .
	mkdir -p dist
	@id=$$(docker create $(BINARY)-builder) && \
	docker cp $$id:/vibecockpit-linux-amd64 dist/ && \
	docker cp $$id:/vibecockpit-linux-arm64 dist/ && \
	docker cp $$id:/vibecockpit-darwin-amd64 dist/ && \
	docker cp $$id:/vibecockpit-darwin-arm64 dist/ && \
	docker rm $$id
	@echo "Binaries in dist/"

dist: docker
