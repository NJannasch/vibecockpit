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
