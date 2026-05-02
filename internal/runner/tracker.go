package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type AgentRun struct {
	TaskID    string    `json:"taskId"`
	TaskTitle string    `json:"taskTitle"`
	BoardName string    `json:"boardName"`
	Project   string    `json:"project"`
	Tool      string    `json:"tool"`
	Model     string    `json:"model"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
	Elapsed   string    `json:"elapsed"`
	Status    string    `json:"status"`
	Iteration int       `json:"iteration"`
	WorkDir   string    `json:"workDir"`
	LogPath   string    `json:"logPath"`
	ExitCode  int       `json:"exitCode,omitempty"`
}

var (
	trackerMu sync.Mutex
	activeRuns = make(map[string]*AgentRun)
)

func trackStart(taskID, taskTitle, boardName, project, tool, model string, pid int, workDir, logPath string) {
	trackerMu.Lock()
	defer trackerMu.Unlock()
	activeRuns[taskID] = &AgentRun{
		TaskID:    taskID,
		TaskTitle: taskTitle,
		BoardName: boardName,
		Project:   project,
		Tool:      tool,
		Model:     model,
		PID:       pid,
		StartedAt: time.Now(),
		Status:    "running",
		WorkDir:   workDir,
		LogPath:   logPath,
	}
}

func trackEnd(taskID string, exitCode int) {
	trackerMu.Lock()
	defer trackerMu.Unlock()
	if r, ok := activeRuns[taskID]; ok {
		if exitCode == 0 {
			r.Status = "completed"
		} else {
			r.Status = "failed"
		}
		r.ExitCode = exitCode
	}
}

func GetActiveRuns() []AgentRun {
	trackerMu.Lock()
	defer trackerMu.Unlock()
	var runs []AgentRun
	for _, r := range activeRuns {
		run := *r
		if run.Status == "running" {
			run.Elapsed = time.Since(run.StartedAt).Truncate(time.Second).String()
		}
		runs = append(runs, run)
	}
	return runs
}

func GetRun(taskID string) *AgentRun {
	trackerMu.Lock()
	defer trackerMu.Unlock()
	if r, ok := activeRuns[taskID]; ok {
		run := *r
		if run.Status == "running" {
			run.Elapsed = time.Since(run.StartedAt).Truncate(time.Second).String()
		}
		return &run
	}
	return nil
}

func StopAgent(taskID string) error {
	trackerMu.Lock()
	r, ok := activeRuns[taskID]
	trackerMu.Unlock()
	if !ok || r.Status != "running" {
		return fmt.Errorf("agent %q not running", taskID)
	}
	proc, err := os.FindProcess(r.PID)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

type AgentLogResponse struct {
	Stdout string `json:"stdout"`
	Status string `json:"status"`
	Recent string `json:"recent"`
}

func GetAgentLog(taskID string) (*AgentLogResponse, error) {
	trackerMu.Lock()
	r, ok := activeRuns[taskID]
	trackerMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("agent %q not found", taskID)
	}

	resp := &AgentLogResponse{}

	if r.LogPath != "" {
		if data, err := os.ReadFile(r.LogPath); err == nil {
			lines := strings.Split(string(data), "\n")
			if len(lines) > 100 {
				lines = lines[len(lines)-100:]
			}
			resp.Stdout = strings.Join(lines, "\n")
		}
	}

	statusPath := filepath.Join(r.WorkDir, ".vibecockpit", "STATUS.md")
	if data, err := os.ReadFile(statusPath); err == nil {
		resp.Status = string(data)
	}

	resp.Recent = readRecentSessionActivity(r.WorkDir)

	return resp, nil
}

func readRecentSessionActivity(workDir string) string {
	home, _ := os.UserHomeDir()
	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}

	var bestPath string
	var bestTime time.Time
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			p := filepath.Join(projectsDir, e.Name())
			info, err := os.Stat(p)
			if err != nil {
				continue
			}
			if info.ModTime().After(bestTime) {
				if data, err := os.ReadFile(filepath.Join(projectsDir, strings.TrimSuffix(e.Name(), ".jsonl"), "session.json")); err == nil {
					if strings.Contains(string(data), workDir) {
						bestPath = p
						bestTime = info.ModTime()
					}
				}
			}
		}
	}

	if bestPath == "" {
		for _, e := range entries {
			if e.IsDir() {
				jsonlPath := filepath.Join(projectsDir, e.Name()+".jsonl")
				info, err := os.Stat(jsonlPath)
				if err != nil {
					continue
				}
				if info.ModTime().After(bestTime) {
					bestPath = jsonlPath
					bestTime = info.ModTime()
				}
			}
		}
	}

	if bestPath == "" {
		return ""
	}

	data, err := os.ReadFile(bestPath)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}

	var summaries []string
	for _, line := range lines {
		var msg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && msg.Role == "assistant" {
			text := msg.Content
			if len(text) > 200 {
				text = text[:200] + "..."
			}
			if text != "" {
				summaries = append(summaries, text)
			}
		}
	}
	if len(summaries) > 3 {
		summaries = summaries[len(summaries)-3:]
	}
	return strings.Join(summaries, "\n---\n")
}
