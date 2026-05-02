package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"vibecockpit/internal/config"
	"vibecockpit/internal/runner"
)

type Job struct {
	ID            string   `yaml:"id" json:"id"`
	Name          string   `yaml:"name" json:"name"`
	Cron          string   `yaml:"cron" json:"cron"`
	Tool          string   `yaml:"tool" json:"tool"`
	Model         string   `yaml:"model,omitempty" json:"model,omitempty"`
	Prompt        string   `yaml:"prompt" json:"prompt"`
	MCPServers    []string `yaml:"mcp_servers,omitempty" json:"mcpServers,omitempty"`
	Board         string   `yaml:"board,omitempty" json:"board,omitempty"`
	Project       string   `yaml:"project,omitempty" json:"project,omitempty"`
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	MaxConcurrent int      `yaml:"max_concurrent,omitempty" json:"maxConcurrent,omitempty"`
	CostCap       float64  `yaml:"cost_cap,omitempty" json:"costCap,omitempty"`
	Permissions   []string `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	LastRun       string   `yaml:"last_run,omitempty" json:"lastRun,omitempty"`
	LastStatus    string   `yaml:"last_status,omitempty" json:"lastStatus,omitempty"`
	NextRun       string   `yaml:"-" json:"nextRun,omitempty"`
	CreatedAt     string   `yaml:"created_at,omitempty" json:"createdAt,omitempty"`
	UpdatedAt     string   `yaml:"updated_at,omitempty" json:"updatedAt,omitempty"`
}

type jobsFile struct {
	Jobs []Job `yaml:"jobs"`
}

type Scheduler struct {
	mu       sync.Mutex
	jobs     []Job
	filePath string
	cfg      *config.Config
	stop     chan struct{}
	running  map[string]bool
}

func New(cfg *config.Config) *Scheduler {
	s := &Scheduler{
		cfg:     cfg,
		stop:    make(chan struct{}),
		running: make(map[string]bool),
	}
	s.filePath = jobsFilePath()
	s.load()
	return s
}

func jobsFilePath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "vibecockpit", "jobs.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vibecockpit", "jobs.yaml")
}

func (s *Scheduler) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	var f jobsFile
	if yaml.Unmarshal(data, &f) == nil {
		s.jobs = f.Jobs
	}
}

func (s *Scheduler) save() error {
	f := jobsFile{Jobs: s.jobs}
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func (s *Scheduler) Start() {
	go s.loop()
}

func (s *Scheduler) Stop() {
	close(s.stop)
}

func (s *Scheduler) loop() {
	s.tick(time.Now())

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			s.tick(now)
		}
	}
}

func (s *Scheduler) tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nowMinute := now.Truncate(time.Minute)

	for i := range s.jobs {
		job := &s.jobs[i]
		if !job.Enabled {
			continue
		}
		if s.running[job.ID] {
			continue
		}
		if !cronMatches(job.Cron, now) {
			continue
		}
		if job.LastRun != "" {
			if last, err := time.Parse(time.RFC3339, job.LastRun); err == nil {
				if !last.Truncate(time.Minute).Before(nowMinute) {
					continue
				}
			}
		}

		s.running[job.ID] = true
		job.LastRun = now.UTC().Format(time.RFC3339)
		job.LastStatus = "running"
		_ = s.save()

		go s.runJob(job.ID)
	}
}

func (s *Scheduler) runJob(jobID string) {
	s.mu.Lock()
	job := s.findJobLocked(jobID)
	if job == nil {
		s.mu.Unlock()
		return
	}
	jobCopy := *job
	s.mu.Unlock()

	taskID := "job-" + jobCopy.ID
	opts := runner.RunOpts{
		TaskID:    taskID,
		BoardName: jobCopy.Board,
		Headless:  true,
	}

	err := runner.RunDirect(s.cfg, opts, runner.DirectTask{
		Title:   jobCopy.Name,
		Prompt:  jobCopy.Prompt,
		Tool:    jobCopy.Tool,
		Model:   jobCopy.Model,
		Project: jobCopy.Project,
	})

	s.mu.Lock()
	defer s.mu.Unlock()

	job = s.findJobLocked(jobID)
	if job == nil {
		delete(s.running, jobID)
		return
	}

	if err != nil {
		job.LastStatus = "failed"
	} else {
		job.LastStatus = "completed"
	}
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	delete(s.running, jobID)
	_ = s.save()
}

func (s *Scheduler) findJobLocked(id string) *Job {
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			return &s.jobs[i]
		}
	}
	return nil
}

// GetJobs returns all jobs with computed NextRun times.
func (s *Scheduler) GetJobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Job, len(s.jobs))
	now := time.Now()
	for i, j := range s.jobs {
		out[i] = j
		if j.Enabled {
			if next, err := NextCronTime(j.Cron, now); err == nil {
				out[i].NextRun = next.UTC().Format(time.RFC3339)
			}
		}
		if s.running[j.ID] {
			out[i].LastStatus = "running"
		}
	}
	return out
}

func (s *Scheduler) GetJob(id string) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.findJobLocked(id)
	if j == nil {
		return nil
	}
	out := *j
	if out.Enabled {
		if next, err := NextCronTime(out.Cron, time.Now()); err == nil {
			out.NextRun = next.UTC().Format(time.RFC3339)
		}
	}
	if s.running[out.ID] {
		out.LastStatus = "running"
	}
	return &out
}

func (s *Scheduler) CreateJob(j Job) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if j.ID == "" {
		j.ID = generateID(j.Name)
	}
	for _, existing := range s.jobs {
		if existing.ID == j.ID {
			return nil, fmt.Errorf("job %q already exists", j.ID)
		}
	}
	if _, err := parseCron(j.Cron); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	if j.Tool == "" {
		j.Tool = "claude"
	}
	if j.MaxConcurrent == 0 {
		j.MaxConcurrent = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	j.CreatedAt = now
	j.UpdatedAt = now

	s.jobs = append(s.jobs, j)
	if err := s.save(); err != nil {
		return nil, err
	}
	return &s.jobs[len(s.jobs)-1], nil
}

func (s *Scheduler) UpdateJob(id string, updates map[string]any) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	j := s.findJobLocked(id)
	if j == nil {
		return nil, fmt.Errorf("job %q not found", id)
	}

	if v, ok := updates["name"].(string); ok && v != "" {
		j.Name = v
	}
	if v, ok := updates["cron"].(string); ok && v != "" {
		if _, err := parseCron(v); err != nil {
			return nil, fmt.Errorf("invalid cron expression: %w", err)
		}
		j.Cron = v
	}
	if v, ok := updates["tool"].(string); ok && v != "" {
		j.Tool = v
	}
	if v, ok := updates["model"].(string); ok {
		j.Model = v
	}
	if v, ok := updates["prompt"].(string); ok && v != "" {
		j.Prompt = v
	}
	if v, ok := updates["board"].(string); ok {
		j.Board = v
	}
	if v, ok := updates["project"].(string); ok {
		j.Project = v
	}
	if v, ok := updates["enabled"].(bool); ok {
		j.Enabled = v
	}
	if v, ok := updates["maxConcurrent"].(float64); ok {
		j.MaxConcurrent = int(v)
	}
	if v, ok := updates["costCap"].(float64); ok {
		j.CostCap = v
	}
	if v, ok := updates["mcpServers"].([]any); ok {
		servers := make([]string, 0, len(v))
		for _, s := range v {
			if str, ok := s.(string); ok {
				servers = append(servers, str)
			}
		}
		j.MCPServers = servers
	}
	if v, ok := updates["permissions"].([]any); ok {
		perms := make([]string, 0, len(v))
		for _, s := range v {
			if str, ok := s.(string); ok {
				perms = append(perms, str)
			}
		}
		j.Permissions = perms
	}
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := s.save(); err != nil {
		return nil, err
	}
	out := *j
	return &out, nil
}

func (s *Scheduler) DeleteJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running[id] {
		return fmt.Errorf("job %q is currently running — stop it first", id)
	}

	for i, j := range s.jobs {
		if j.ID == id {
			s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("job %q not found", id)
}

func (s *Scheduler) TriggerJob(id string) error {
	s.mu.Lock()
	if s.running[id] {
		s.mu.Unlock()
		return fmt.Errorf("job %q is already running", id)
	}
	j := s.findJobLocked(id)
	if j == nil {
		s.mu.Unlock()
		return fmt.Errorf("job %q not found", id)
	}

	s.running[id] = true
	j.LastRun = time.Now().UTC().Format(time.RFC3339)
	j.LastStatus = "running"
	_ = s.save()
	s.mu.Unlock()

	go s.runJob(id)
	return nil
}

func (s *Scheduler) PauseJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j := s.findJobLocked(id)
	if j == nil {
		return fmt.Errorf("job %q not found", id)
	}
	j.Enabled = false
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.save()
}

func (s *Scheduler) ResumeJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j := s.findJobLocked(id)
	if j == nil {
		return fmt.Errorf("job %q not found", id)
	}
	j.Enabled = true
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.save()
}

func (s *Scheduler) CancelJob(id string) error {
	s.mu.Lock()
	if !s.running[id] {
		s.mu.Unlock()
		return fmt.Errorf("job %q is not running", id)
	}
	s.mu.Unlock()

	taskID := "job-" + id
	if err := runner.StopAgent(taskID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, id)
	j := s.findJobLocked(id)
	if j != nil {
		j.LastStatus = "cancelled"
		j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return s.save()
}

func (s *Scheduler) IsRunning(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[id]
}

func (s *Scheduler) GetJobRuns(jobID string) []runner.AgentRun {
	taskID := "job-" + jobID
	all := runner.GetActiveRuns()
	var runs []runner.AgentRun
	for _, r := range all {
		if r.TaskID == taskID {
			runs = append(runs, r)
		}
	}
	return runs
}

// SetFilePath overrides the default jobs file path (for testing).
func (s *Scheduler) SetFilePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filePath = path
	s.jobs = nil
	s.load()
}

// -- Cron parsing --

type cronExpr struct {
	minutes  []int
	hours    []int
	days     []int
	months   []int
	weekdays []int
}

func parseCron(expr string) (*cronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	minutes, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	hours, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	days, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day: %w", err)
	}
	months, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	weekdays, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("weekday: %w", err)
	}

	return &cronExpr{
		minutes:  minutes,
		hours:    hours,
		days:     days,
		months:   months,
		weekdays: weekdays,
	}, nil
}

func parseField(field string, min, max int) ([]int, error) {
	if field == "*" {
		return nil, nil // nil means "all"
	}

	// Handle */step
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step: %s", field)
		}
		var vals []int
		for i := min; i <= max; i += step {
			vals = append(vals, i)
		}
		return vals, nil
	}

	// Handle comma-separated values and ranges
	var vals []int
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || lo < min || hi > max || lo > hi {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			for i := lo; i <= hi; i++ {
				vals = append(vals, i)
			}
		} else {
			v, err := strconv.Atoi(part)
			if err != nil || v < min || v > max {
				return nil, fmt.Errorf("invalid value: %s", part)
			}
			vals = append(vals, v)
		}
	}
	return vals, nil
}

func contains(vals []int, v int) bool {
	if vals == nil {
		return true // nil = wildcard
	}
	for _, x := range vals {
		if x == v {
			return true
		}
	}
	return false
}

func cronMatches(expr string, t time.Time) bool {
	c, err := parseCron(expr)
	if err != nil {
		return false
	}
	return contains(c.minutes, t.Minute()) &&
		contains(c.hours, t.Hour()) &&
		contains(c.days, t.Day()) &&
		contains(c.months, int(t.Month())) &&
		contains(c.weekdays, int(t.Weekday()))
}

// NextCronTime computes the next fire time after `after`.
func NextCronTime(expr string, after time.Time) (time.Time, error) {
	c, err := parseCron(expr)
	if err != nil {
		return time.Time{}, err
	}

	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.Add(366 * 24 * time.Hour)

	for t.Before(limit) {
		if contains(c.months, int(t.Month())) &&
			contains(c.days, t.Day()) &&
			contains(c.weekdays, int(t.Weekday())) &&
			contains(c.hours, t.Hour()) &&
			contains(c.minutes, t.Minute()) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no next run found within a year")
}

// CronToHuman converts a 5-field cron expression to a human-readable string.
func CronToHuman(expr string) string {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return expr
	}
	min, hour, dom, _, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	dayNames := map[string]string{"0": "Sunday", "1": "Monday", "2": "Tuesday", "3": "Wednesday", "4": "Thursday", "5": "Friday", "6": "Saturday"}

	switch {
	case min == "*" && hour == "*":
		return "Every minute"
	case strings.HasPrefix(min, "*/"):
		return "Every " + min[2:] + " minutes"
	case strings.HasPrefix(hour, "*/"):
		return "Every " + hour[2:] + " hours"
	case dow != "*" && dom == "*":
		dayName := dayNames[dow]
		if dayName == "" {
			dayName = "day " + dow
		}
		return fmt.Sprintf("Every %s at %s:%s", dayName, hour, padZero(min))
	case dom == "*" && dow == "*":
		return fmt.Sprintf("Daily at %s:%s", hour, padZero(min))
	default:
		return expr
	}
}

func padZero(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func generateID(name string) string {
	id := strings.ToLower(name)
	id = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, id)
	for strings.Contains(id, "--") {
		id = strings.ReplaceAll(id, "--", "-")
	}
	id = strings.Trim(id, "-")
	if len(id) > 40 {
		id = id[:40]
	}
	if id == "" {
		id = "job"
	}
	return id
}

// MarshalJSON implements custom JSON for Job to include humanCron.
func (j Job) MarshalJSON() ([]byte, error) {
	type Alias Job
	return json.Marshal(struct {
		Alias
		HumanCron string `json:"humanCron,omitempty"`
	}{
		Alias:     Alias(j),
		HumanCron: CronToHuman(j.Cron),
	})
}
