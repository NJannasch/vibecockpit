FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ .
COPY internal/web/static/ ../internal/web/static/
RUN npm run build

FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/internal/web/static/ ./internal/web/static/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/vibecockpit-linux-amd64 .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /out/vibecockpit-linux-arm64 .
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o /out/vibecockpit-darwin-amd64 .
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o /out/vibecockpit-darwin-arm64 .

FROM scratch
COPY --from=builder /out/ /
