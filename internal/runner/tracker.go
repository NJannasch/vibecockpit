package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	Source    string    `json:"source,omitempty"`
	Cost      float64   `json:"cost,omitempty"`
	Duration  float64   `json:"durationSec,omitempty"`
}

var (
	trackerMu    sync.Mutex
	activeRuns   = make(map[string]*AgentRun)
	persistPath  string
)

func init() {
	home, _ := os.UserHomeDir()
	persistPath = filepath.Join(home, ".config", "vibecockpit", "agents.json")
	trackerMu.Lock()
	defer trackerMu.Unlock()
	data, err := os.ReadFile(persistPath)
	if err != nil {
		return
	}
	var runs map[string]*AgentRun
	if json.Unmarshal(data, &runs) == nil {
		for id, r := range runs {
			if r.Status == "running" && !isProcessAlive(r.PID) {
				r.Status = "failed"
				r.ExitCode = -1
			}
			activeRuns[id] = r
		}
	}
}

func persistRuns() {
	data, err := json.Marshal(activeRuns)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(persistPath), 0755)
	_ = os.WriteFile(persistPath, data, 0644)
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

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
	persistRuns()
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
		r.Duration = time.Since(r.StartedAt).Seconds()
		r.Elapsed = time.Since(r.StartedAt).Truncate(time.Second).String()
		persistRuns()
	}
}

func runSource(taskID string) string {
	if strings.HasPrefix(taskID, "job-") {
		return "scheduled"
	}
	if strings.HasPrefix(taskID, "quick-") {
		return "quick"
	}
	return "task"
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
		run.Source = runSource(run.TaskID)
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
		run.Source = runSource(run.TaskID)
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

func readRecentSessionActivity(_ string) string {
	home, _ := os.UserHomeDir()
	projectsDir := filepath.Join(home, ".claude", "projects")

	// The workDir is a worktree path like .../place2be/.vibecockpit/worktrees/task-id
	// Claude stores projects by encoded path, so look for dirs containing the worktree path

	var candidates []string
	entries, _ := os.ReadDir(projectsDir)
	for _, e := range entries {
		name := e.Name()
		// Match by encoded worktree path in the directory/file name
		if strings.Contains(name, ".vibecockpit") || strings.Contains(name, "worktrees") {
			jsonlPath := filepath.Join(projectsDir, name)
			if strings.HasSuffix(name, ".jsonl") {
				candidates = append(candidates, jsonlPath)
			} else if e.IsDir() {
				jsonlPath = filepath.Join(projectsDir, name+".jsonl")
				if _, err := os.Stat(jsonlPath); err == nil {
					candidates = append(candidates, jsonlPath)
				}
			}
		}
	}

	// Fallback: find most recently modified JONLs
	if len(candidates) == 0 {
		cutoff := time.Now().Add(-10 * time.Minute)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				p := filepath.Join(projectsDir, e.Name())
				info, err := os.Stat(p)
				if err == nil && info.ModTime().After(cutoff) {
					candidates = append(candidates, p)
				}
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		infoI, _ := os.Stat(candidates[i])
		infoJ, _ := os.Stat(candidates[j])
		if infoI == nil || infoJ == nil {
			return false
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})

	for _, path := range candidates {
		content := tailFile(path, 8192)
		if content == "" {
			continue
		}
		lines := strings.Split(strings.TrimSpace(content), "\n")
		var summaries []string
		for _, line := range lines {
			var msg struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			}
			if json.Unmarshal([]byte(line), &msg) != nil || msg.Role != "assistant" {
				continue
			}
			text := extractText(msg.Content)
			if text == "" {
				continue
			}
			if len(text) > 200 {
				text = text[:200] + "..."
			}
			summaries = append(summaries, text)
		}
		if len(summaries) > 0 {
			if len(summaries) > 3 {
				summaries = summaries[len(summaries)-3:]
			}
			return strings.Join(summaries, "\n---\n")
		}
	}
	return ""
}

func tailFile(path string, bytes int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, _ := f.Stat()
	if info.Size() <= bytes {
		data, _ := os.ReadFile(path)
		return string(data)
	}
	buf := make([]byte, bytes)
	_, _ = f.Seek(-bytes, 2)
	n, _ := f.Read(buf)
	content := string(buf[:n])
	// Skip partial first line
	if idx := strings.Index(content, "\n"); idx >= 0 {
		content = content[idx+1:]
	}
	return content
}

func GetAgentDiff(taskID string) (string, error) {
	trackerMu.Lock()
	r, ok := activeRuns[taskID]
	trackerMu.Unlock()
	if !ok {
		return "", fmt.Errorf("agent %q not found", taskID)
	}

	branch := "vibecockpit/" + taskID
	// Find the project root (parent of .vibecockpit/worktrees/)
	projectDir := r.WorkDir
	if idx := strings.Index(projectDir, "/.vibecockpit/worktrees/"); idx >= 0 {
		projectDir = projectDir[:idx]
	}

	cmd := exec.Command("git", "diff", "HEAD..."+branch)
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

func MergeAgentBranch(taskID string) error {
	trackerMu.Lock()
	r, ok := activeRuns[taskID]
	trackerMu.Unlock()
	if !ok {
		return fmt.Errorf("agent %q not found", taskID)
	}

	branch := "vibecockpit/" + taskID
	projectDir := r.WorkDir
	if idx := strings.Index(projectDir, "/.vibecockpit/worktrees/"); idx >= 0 {
		projectDir = projectDir[:idx]
	}

	cmd := exec.Command("git", "merge", "--no-ff", "-m", "Merge agent work: "+taskID, branch)
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}

	// Clean up the branch after merge
	delCmd := exec.Command("git", "branch", "-d", branch)
	delCmd.Dir = projectDir
	_ = delCmd.Run()

	return nil
}

func SetRunCost(taskID string, cost float64) {
	trackerMu.Lock()
	defer trackerMu.Unlock()
	if r, ok := activeRuns[taskID]; ok {
		r.Cost = cost
		persistRuns()
	}
}

func DeleteRun(taskID string, cleanupBranch bool) error {
	trackerMu.Lock()
	r, ok := activeRuns[taskID]
	if ok && r.Status == "running" {
		trackerMu.Unlock()
		return fmt.Errorf("cannot delete running agent — stop it first")
	}
	if ok && cleanupBranch {
		projectDir := r.WorkDir
		if idx := strings.Index(projectDir, "/.vibecockpit/worktrees/"); idx >= 0 {
			projectDir = projectDir[:idx]
		}
		branch := "vibecockpit/" + taskID
		cmd := exec.Command("git", "branch", "-D", branch)
		cmd.Dir = projectDir
		_ = cmd.Run()
	}
	delete(activeRuns, taskID)
	persistRuns()
	trackerMu.Unlock()
	return nil
}

func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var texts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						texts = append(texts, t)
					}
				}
			}
		}
		return strings.Join(texts, " ")
	}
	return ""
}
