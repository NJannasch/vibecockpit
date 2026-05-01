package audit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	Timestamp  string `json:"timestamp"`
	Tool       string `json:"tool"`
	Params     any    `json:"params,omitempty"`
	ResultHash string `json:"resultHash"`
	ResultCount int   `json:"resultCount"`
}

type Logger struct {
	mu   sync.Mutex
	path string
}

func NewLogger() *Logger {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return &Logger{
		path: filepath.Join(dir, "vibecockpit", "mcp-audit.jsonl"),
	}
}

func (l *Logger) Log(tool string, params any, result []byte, count int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	h := sha256.Sum256(result)

	entry := Entry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Tool:        tool,
		Params:      params,
		ResultHash:  fmt.Sprintf("%x", h[:8]),
		ResultCount: count,
	}

	_ = os.MkdirAll(filepath.Dir(l.path), 0755)
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	data, _ := json.Marshal(entry)
	_, _ = f.Write(data)
	_, _ = f.Write([]byte("\n"))
}

func (l *Logger) Path() string {
	return l.path
}

func (l *Logger) ReadLog(limit int) ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []Entry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	// Return most recent first
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
