package scanner

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"vibecockpit/internal/provider"
	"vibecockpit/internal/sanitize"
)

type Finding struct {
	SessionID   string `json:"sessionId"`
	Provider    string `json:"provider"`
	ProjectName string `json:"projectName"`
	RuleID      string `json:"ruleId"`
	Match       string `json:"match"`
	Line        int    `json:"line"`
	Timestamp   string `json:"timestamp"`
}

type Status struct {
	State          string    `json:"state"` // "idle", "scanning", "done"
	StartedAt      string    `json:"startedAt,omitempty"`
	CompletedAt    string    `json:"completedAt,omitempty"`
	SessionsTotal  int       `json:"sessionsTotal"`
	SessionsDone   int       `json:"sessionsDone"`
	FilesScanned   int       `json:"filesScanned"`
	LinesScanned   int       `json:"linesScanned"`
	FindingCount   int       `json:"findingCount"`
	Findings       []Finding `json:"findings"`
	PatternsLoaded int       `json:"patternsLoaded"`
	DurationMs     int64     `json:"durationMs"`
	CurrentFile    string    `json:"currentFile,omitempty"`
}

type Config struct {
	SkipRules  []string
	ExtraHints []string
}

type Scanner struct {
	providers []provider.Provider
	cfg       Config
	rules     []rule

	mu     sync.RWMutex
	status Status
	cancel context.CancelFunc
}

// A rule has keywords (fast string check) and a regex (slow but precise).
// The regex only runs if ALL keywords match the line — same as gitleaks.
type rule struct {
	id       string
	keywords []string // lowercase; ALL must match for regex to fire
	pattern  *regexp.Regexp
}

// Rules that are too noisy for AI session logs
var skipRule = map[string]bool{
	"generic-api-key":  true,
	"cloudflare-api-key": true, // matches MCP tool names like mcp__cloudflare__*
}

var knownTestValues = []string{
	"AKIATEST", "AKIAIOSFODNN7EXAMPLE",
	"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	"sk-test", "pk-test", "YOUR_API_KEY", "your_api_key",
}

func New(providers []provider.Provider, cfg ...Config) *Scanner {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &Scanner{
		providers: providers,
		cfg:       c,
		status:    Status{State: "idle"},
	}
}

func (s *Scanner) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *Scanner) Start() {
	s.mu.Lock()
	if s.status.State == "scanning" {
		s.mu.Unlock()
		return
	}
	s.loadRules()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.status = Status{
		State:          "scanning",
		StartedAt:      time.Now().UTC().Format(time.RFC3339),
		PatternsLoaded: len(s.rules),
	}
	s.mu.Unlock()
	go s.run(ctx)
}

func (s *Scanner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Scanner) run(ctx context.Context) {
	start := time.Now()

	var allSessions []provider.Session
	for _, p := range s.providers {
		sessions, err := p.ScanSessions(ctx)
		if err != nil {
			continue
		}
		allSessions = append(allSessions, sessions...)
	}

	s.mu.Lock()
	s.status.SessionsTotal = len(allSessions)
	s.mu.Unlock()

	for i, sess := range allSessions {
		select {
		case <-ctx.Done():
			s.finish(start)
			return
		default:
		}

		s.mu.Lock()
		s.status.SessionsDone = i
		s.status.CurrentFile = sess.ProjectName + " (" + sess.Provider + ")"
		s.mu.Unlock()

		s.scanSession(ctx, sess)
	}

	s.mu.Lock()
	s.status.SessionsDone = len(allSessions)
	s.mu.Unlock()
	s.finish(start)
}

func (s *Scanner) finish(start time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.State = "done"
	s.status.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	s.status.DurationMs = time.Since(start).Milliseconds()
	s.status.CurrentFile = ""
}

func (s *Scanner) loadRules() {
	if len(s.rules) > 0 {
		return
	}
	_ = sanitize.PatternCount()
	s.rules = s.parseGitleaksRules()

	// Extra patterns not in gitleaks
	extras := []struct {
		id, kw, pattern string
	}{
		{"private-key", "begin", `-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`},
		{"connection-string", "://", `(?i)(mongodb|postgres|mysql|redis)://\S+:\S+@\S+`},
	}
	for _, e := range extras {
		if re, err := regexp.Compile(e.pattern); err == nil {
			s.rules = append(s.rules, rule{id: e.id, keywords: []string{e.kw}, pattern: re})
		}
	}
}

func (s *Scanner) isSkipped(id string) bool {
	if skipRule[id] {
		return true
	}
	for _, skip := range s.cfg.SkipRules {
		if skip == id {
			return true
		}
	}
	return false
}

func (s *Scanner) parseGitleaksRules() []rule {
	path := sanitize.CachePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var rules []rule
	var currentID string
	var currentKeywords []string

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if line == "[[rules]]" {
			currentID = ""
			currentKeywords = nil
			continue
		}

		// Parse id = "..."
		if strings.HasPrefix(line, "id") && !strings.HasPrefix(line, "id_") && strings.Contains(line, "=") {
			idx := strings.Index(line, "=")
			currentID = strings.TrimSpace(line[idx+1:])
			currentID = strings.Trim(currentID, `"'`)
			continue
		}

		// Parse keywords = ["word1", "word2"]
		if strings.HasPrefix(line, "keywords") && strings.Contains(line, "=") {
			idx := strings.Index(line, "=")
			val := strings.TrimSpace(line[idx+1:])
			currentKeywords = parseKeywords(val)
			continue
		}

		// Parse regex = '''...''' or regex = "..."
		if !strings.HasPrefix(line, "regex") || strings.HasPrefix(line, "regex_") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		val := strings.TrimSpace(line[idx+1:])

		var raw string
		if strings.HasPrefix(val, "'''") && strings.HasSuffix(val, "'''") {
			raw = val[3 : len(val)-3]
		} else if strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) {
			raw = val[1 : len(val)-1]
			raw = strings.ReplaceAll(raw, `\"`, `"`)
			raw = strings.ReplaceAll(raw, `\\`, `\`)
		} else {
			continue
		}
		if raw == "" {
			continue
		}

		id := currentID
		if id == "" {
			id = "unknown"
		}
		if s.isSkipped(id) {
			continue
		}

		re, err := regexp.Compile(raw)
		if err != nil {
			continue
		}

		kw := currentKeywords
		if len(kw) == 0 {
			// No keywords defined — extract a literal hint from the regex
			kw = extractHintFromRegex(raw)
		}

		rules = append(rules, rule{id: id, keywords: kw, pattern: re})
	}
	return rules
}

func parseKeywords(val string) []string {
	// keywords = ["word1", "word2"]
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")
	var kws []string
	for _, part := range strings.Split(val, ",") {
		kw := strings.TrimSpace(part)
		kw = strings.Trim(kw, `"'`)
		kw = strings.ToLower(kw)
		if kw != "" {
			kws = append(kws, kw)
		}
	}
	return kws
}

func extractHintFromRegex(raw string) []string {
	// Try to find a literal string prefix in the regex for fast matching
	// Look for sequences of 3+ alphanumeric chars
	var best string
	current := ""
	for _, c := range raw {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			current += string(c)
		} else {
			if len(current) > len(best) {
				best = current
			}
			current = ""
		}
	}
	if len(current) > len(best) {
		best = current
	}
	if len(best) >= 3 {
		return []string{strings.ToLower(best)}
	}
	return nil
}

func isTestValue(match string) bool {
	lower := strings.ToLower(match)
	for _, tv := range knownTestValues {
		if strings.Contains(lower, strings.ToLower(tv)) {
			return true
		}
	}
	return false
}

func (s *Scanner) scanSession(ctx context.Context, sess provider.Session) {
	if sess.DataPath == "" {
		return
	}
	switch {
	case strings.HasSuffix(sess.DataPath, ".jsonl"),
		strings.HasSuffix(sess.DataPath, ".json"),
		strings.HasSuffix(sess.DataPath, ".yaml"):
		s.scanFile(ctx, sess, sess.DataPath)
	}
}

func (s *Scanner) scanFile(ctx context.Context, sess provider.Session, path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	s.mu.Lock()
	s.status.FilesScanned++
	s.mu.Unlock()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 512*1024), 512*1024)
	lineNum := 0

	for sc.Scan() {
		lineNum++

		if lineNum%1000 == 0 {
			s.mu.Lock()
			s.status.LinesScanned += 1000
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			default:
			}
		}

		line := sc.Text()
		lineLower := strings.ToLower(line)

		// For each rule: check keywords first (fast), then regex (slow)
		for _, r := range s.rules {
			if !keywordsMatch(lineLower, r.keywords) {
				continue
			}

			match := r.pattern.FindString(line)
			if match == "" || len(match) < 8 || isTestValue(match) {
				continue
			}

			finding := Finding{
				SessionID:   sess.ID,
				Provider:    sess.Provider,
				ProjectName: sess.ProjectName,
				RuleID:      r.id,
				Match:       redactMatch(match),
				Line:        lineNum,
				Timestamp:   time.Now().UTC().Format(time.RFC3339),
			}

			s.mu.Lock()
			s.status.Findings = append(s.status.Findings, finding)
			s.status.FindingCount = len(s.status.Findings)
			s.mu.Unlock()
		}
	}

	// Add remaining lines to count
	s.mu.Lock()
	s.status.LinesScanned += lineNum % 1000
	s.mu.Unlock()
}

// keywordsMatch returns true if ALL keywords appear in the lowercase line.
// If no keywords are defined, the rule always fires (expensive but rare).
func keywordsMatch(lineLower string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	for _, kw := range keywords {
		if !strings.Contains(lineLower, kw) {
			return false
		}
	}
	return true
}

func redactMatch(match string) string {
	n := len(match)
	if n <= 8 {
		return match[:2] + strings.Repeat("*", n-2)
	}
	mid := n / 2
	return match[:mid-2] + "****" + match[mid+2:]
}
