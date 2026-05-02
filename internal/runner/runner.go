package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"vibecockpit/internal/board"
	"vibecockpit/internal/config"
)

type RunOpts struct {
	TaskID    string
	BoardName string
	Isolation string // "none", "worktree"
	Headless  bool   // true when spawned from web UI (no stdin, log to file)
}

type toolConfig struct {
	bin       string
	args      []string
	mcpFile   string
	mcpKey    string
	permFile  string
}

func Run(cfg *config.Config, opts RunOpts) error {
	boards, err := board.Discover(cfg.NewProjectDir)
	if err != nil {
		return fmt.Errorf("discover boards: %w", err)
	}

	var b *board.Board
	var task *board.Task
	if opts.BoardName != "" {
		b = board.FindBoard(boards, opts.BoardName)
		if b == nil {
			return fmt.Errorf("board %q not found", opts.BoardName)
		}
		task, _ = b.FindTask(opts.TaskID)
	} else {
		for _, bd := range boards {
			if t, _ := bd.FindTask(opts.TaskID); t != nil {
				b = bd
				task = t
				break
			}
		}
	}
	if task == nil {
		return fmt.Errorf("task %q not found", opts.TaskID)
	}

	tool := task.Tool
	if tool == "" {
		tool = b.Defaults.Tool
	}
	if tool == "" {
		tool = "claude"
	}

	model := task.Model
	if model == "" {
		model = b.Defaults.Model
	}

	projectDir := expandHome(b.Project)

	workDir := projectDir
	var worktreePath string
	if isGitRepo(projectDir) {
		var err error
		worktreePath, err = createWorktree(projectDir, task.ID)
		if err != nil {
			return fmt.Errorf("create worktree: %w", err)
		}
		workDir = worktreePath
		defer cleanupWorktree(projectDir, worktreePath, task.ID)
	}

	prompt := composePrompt(task, workDir)
	tc := toolConfigFor(tool)

	binPath := resolveBin(cfg, tool, tc.bin)
	if binPath == "" {
		return fmt.Errorf("could not find %q in PATH — configure in settings provider_paths", tc.bin)
	}

	if err := ensureMCPConfig(workDir, tc, task.MCP); err != nil {
		return fmt.Errorf("write MCP config: %w", err)
	}

	args := buildArgs(tc, model, prompt)

	fmt.Printf("Spawning %s on task %q\n", tool, task.ID)
	fmt.Printf("  Project: %s\n", workDir)
	if worktreePath != "" {
		fmt.Printf("  Isolation: worktree\n")
	} else {
		fmt.Printf("  Isolation: none (not a git repo)\n")
	}
	fmt.Printf("  Model:   %s\n", model)
	fmt.Printf("  Prompt:  %s\n", truncate(prompt, 80))

	if err := b.MoveTaskBy(task.ID, "in-progress", "vibecockpit"); err != nil {
		return err
	}
	if err := b.Save(); err != nil {
		return err
	}

	maxIter := task.MaxIterations
	if maxIter <= 0 {
		maxIter = 1
	}

	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("vibecockpit-agent-%s.log", task.ID))
	var lastFailOutput string

	for iter := 1; iter <= maxIter; iter++ {
		if iter > 1 {
			fmt.Fprintf(os.Stderr, "\n=== Iteration %d/%d ===\n", iter, maxIter)
			retryPrompt := prompt + "\n\nPrevious attempt failed acceptance checks:\n" + lastFailOutput + "\nFix the issues and retry."
			args = buildArgs(tc, model, retryPrompt)
		}

		task.Iterations = iter
		_ = b.Save()

		cmd := exec.Command(binPath, args...)
		cmd.Dir = workDir
		cmd.Env = buildEnv(cfg, binPath, task.ID)

		if opts.Headless {
			logFile, err := os.Create(logPath)
			if err != nil {
				return fmt.Errorf("create log file: %w", err)
			}
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			cmd.Stdin = nil
			if err := cmd.Start(); err != nil {
				logFile.Close()
				return fmt.Errorf("start agent: %w", err)
			}
			trackStart(task.ID, task.Title, b.Name, b.Project, tool, model, cmd.Process.Pid, workDir, logPath)
			err = cmd.Wait()
			logFile.Close()
		} else {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("start agent: %w", err)
			}
			trackStart(task.ID, task.Title, b.Name, b.Project, tool, model, cmd.Process.Pid, workDir, logPath)
			err = cmd.Wait()
		}

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
			fmt.Fprintf(os.Stderr, "Agent %s iteration %d exited with error: %v\n", task.ID, iter, err)
		}
		trackEnd(task.ID, exitCode)

		if maxIter <= 1 {
			break
		}

		// Check automated acceptance criteria
		failed, output := checkAcceptanceCriteria(task, workDir)
		if len(failed) == 0 {
			fmt.Fprintf(os.Stderr, "All acceptance criteria passed on iteration %d\n", iter)
			break
		}

		lastFailOutput = output
		if iter == maxIter {
			fmt.Fprintf(os.Stderr, "Max iterations (%d) reached. Moving to review.\n", maxIter)
		}
	}

	_ = b.MoveTaskBy(task.ID, "review", "vibecockpit")
	_ = b.Save()

	return nil
}

var instructionFiles = []string{
	"CLAUDE.md", "AGENTS.md", "GEMINI.md", "OPENCODE.md",
	".cursorrules", "codex.md", "CODEX.md",
	".github/copilot-instructions.md",
	".windsurfrules",
}

func composePrompt(t *board.Task, projectDir string) string {
	var parts []string

	var found []string
	for _, f := range instructionFiles {
		if _, err := os.Stat(filepath.Join(projectDir, f)); err == nil {
			found = append(found, f)
		}
	}
	if len(found) > 0 {
		parts = append(parts, "Read "+strings.Join(found, " and ")+" first for project conventions.\n")
	}

	parts = append(parts, "IMPORTANT: Write your progress to .vibecockpit/STATUS.md as you work. Include: what you've done, what's left, any blockers. Update it before finishing.\n")

	parts = append(parts, t.Title)
	if t.Description != "" {
		parts = append(parts, "\n"+t.Description)
	}
	if len(t.Acceptance) > 0 {
		parts = append(parts, "\nAcceptance criteria:")
		for _, a := range t.Acceptance {
			parts = append(parts, "- "+a)
		}
	}
	return strings.Join(parts, "\n")
}

func toolConfigFor(tool string) toolConfig {
	switch tool {
	case "claude":
		return toolConfig{
			bin:      "claude",
			mcpFile:  ".mcp.json",
			mcpKey:   "mcpServers",
			permFile: ".claude/settings.local.json",
		}
	case "codex":
		return toolConfig{
			bin:    "codex",
			mcpFile: "codex.json",
			mcpKey:  "mcpServers",
		}
	case "gemini":
		return toolConfig{
			bin:    "gemini",
			mcpFile: ".gemini/settings.json",
			mcpKey:  "mcpServers",
		}
	case "opencode":
		return toolConfig{
			bin:    "opencode",
			mcpFile: "opencode.json",
			mcpKey:  "mcp",
		}
	default:
		return toolConfig{bin: tool, mcpFile: ".mcp.json", mcpKey: "mcpServers"}
	}
}

func buildArgs(tc toolConfig, model, prompt string) []string {
	switch tc.bin {
	case "claude":
		args := []string{"-p", prompt, "--permission-mode", "dontAsk", "--allowedTools"}
		args = append(args, defaultAllowedTools()...)
		if model != "" {
			args = append(args, "--model", model)
		}
		return args
	case "codex":
		args := []string{"exec"}
		if model != "" {
			args = append(args, "--model", model)
		}
		args = append(args, prompt)
		return args
	case "gemini":
		args := []string{"--yolo", "-p", prompt}
		if model != "" {
			args = append(args, "--model", model)
		}
		return args
	case "opencode":
		args := []string{"run"}
		if model != "" {
			args = append(args, "--model", model)
		}
		args = append(args, prompt)
		return args
	default:
		return []string{prompt}
	}
}

func ensureMCPConfig(projectDir string, tc toolConfig, mcpServers []string) error {
	mcpPath := filepath.Join(projectDir, tc.mcpFile)

	existing := make(map[string]any)
	if data, err := os.ReadFile(mcpPath); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	servers, ok := existing[tc.mcpKey].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}

	if _, exists := servers["vibecockpit"]; !exists {
		servers["vibecockpit"] = map[string]any{
			"command": "vibecockpit",
			"args":    []string{"--mcp"},
		}
	}

	existing[tc.mcpKey] = servers

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(mcpPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(mcpPath, append(data, '\n'), 0644)
}

func buildEnv(cfg *config.Config, binPath, taskID string) []string {
	env := os.Environ()

	env = append(env, "VIBECOCKPIT_TASK="+taskID)

	var prepend []string
	for _, p := range cfg.ExtraPath {
		prepend = append(prepend, expandHome(p))
	}
	binDir := filepath.Dir(binPath)
	if binDir != "." && binDir != "" {
		prepend = append(prepend, binDir)
	}

	if len(prepend) > 0 {
		extra := strings.Join(prepend, string(os.PathListSeparator))
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + extra + string(os.PathListSeparator) + e[5:]
				break
			}
		}
	}

	return env
}

func resolveBin(cfg *config.Config, provName, bin string) string {
	if p, ok := cfg.ProviderPaths[provName]; ok {
		p = expandHome(p)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath(bin); err == nil {
		return p
	}
	return ""
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return home + path[1:]
	}
	return path
}

func hasActiveSessions(projectDir string) bool {
	out, err := exec.Command("pgrep", "-a", "claude|codex|gemini|opencode").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), projectDir)
}

func createWorktree(projectDir, taskID string) (string, error) {
	worktreeDir := filepath.Join(projectDir, ".vibecockpit", "worktrees", taskID)
	branch := "vibecockpit/" + taskID

	// Clean up previous run if exists
	rmCmd := exec.Command("git", "worktree", "remove", "--force", worktreeDir)
	rmCmd.Dir = projectDir
	_ = rmCmd.Run()
	delCmd := exec.Command("git", "branch", "-D", branch)
	delCmd.Dir = projectDir
	_ = delCmd.Run()

	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0755); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreeDir, "HEAD")
	cmd.Dir = projectDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return worktreeDir, nil
}

func cleanupWorktree(projectDir, worktreePath, taskID string) {
	cmd := exec.Command("git", "worktree", "remove", worktreePath)
	cmd.Dir = projectDir
	_ = cmd.Run()

	branch := "vibecockpit/" + taskID
	fmt.Printf("Worktree removed. Branch %q preserved.\n", branch)
	fmt.Printf("  Review: git diff main...%s\n", branch)
	fmt.Printf("  Merge:  git merge %s\n", branch)
	fmt.Printf("  Delete: git branch -D %s\n", branch)
}

func checkAcceptanceCriteria(t *board.Task, workDir string) (failed []string, output string) {
	var outputs []string
	for _, criterion := range t.Acceptance {
		if !strings.HasPrefix(criterion, "run:") {
			continue
		}
		cmdStr := strings.TrimSpace(strings.TrimPrefix(criterion, "run:"))
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			continue
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			failed = append(failed, criterion)
			outputs = append(outputs, fmt.Sprintf("FAIL: %s\n%s", cmdStr, string(out)))
		} else {
			outputs = append(outputs, fmt.Sprintf("PASS: %s", cmdStr))
		}
	}
	return failed, strings.Join(outputs, "\n")
}

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func defaultAllowedTools() []string {
	return []string{
		"Read", "Edit", "Write",
		"Bash(git *)", "Bash(ls *)", "Bash(find *)", "Bash(grep *)", "Bash(cat *)",
		"Bash(go *)", "Bash(npm *)", "Bash(npx *)", "Bash(make *)",
		"Bash(python *)", "Bash(pip *)", "Bash(cargo *)", "Bash(rustc *)",
		"Bash(node *)", "Bash(bun *)", "Bash(pnpm *)", "Bash(yarn *)",
		"Bash(test *)", "Bash(echo *)", "Bash(mkdir *)", "Bash(cp *)", "Bash(mv *)",
		"mcp__vibecockpit",
	}
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
