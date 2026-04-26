package sanitize

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	rulesURL = "https://raw.githubusercontent.com/NJannasch/vibecockpit/main/internal/sanitize/gitleaks.toml"
	maxAge   = 7 * 24 * time.Hour
	redacted = "[REDACTED]"
)

var (
	patternsOnce sync.Once
	patterns     []*regexp.Regexp
)

var fallbackPatterns = []string{
	`(?i)sk-[a-zA-Z0-9]{20,}`,
	`(?i)ghp_[a-zA-Z0-9]{36,}`,
	`(?i)gho_[a-zA-Z0-9]{36,}`,
	`(?i)glpat-[a-zA-Z0-9\-]{20,}`,
	`(?i)xox[bpars]-[a-zA-Z0-9\-]{10,}`,
	`(?i)Bearer\s+[a-zA-Z0-9\-._~+/]+=*`,
	`(?i)AKIA[0-9A-Z]{16}`,
	`(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`,
	`(?i)export\s+\w+=\S{8,}`,
	`(?i)(password|passwd|secret|token|api_key)\s*[:=]\s*\S{6,}`,
}

func cachePath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "vibecockpit", "gitleaks.toml")
}

func loadPatterns() {
	patternsOnce.Do(func() {
		config := loadRules()
		if config != "" {
			patterns = parseGitleaksPatterns(config)
		}

		// Always add fallback patterns (in case gitleaks parse yields few results)
		for _, raw := range fallbackPatterns {
			if re, err := regexp.Compile(raw); err == nil {
				patterns = append(patterns, re)
			}
		}
	})
}

func loadRules() string {
	path := cachePath()

	// Check if cached file exists and is fresh
	if info, err := os.Stat(path); err == nil {
		if time.Since(info.ModTime()) < maxAge {
			if data, err := os.ReadFile(path); err == nil {
				return string(data)
			}
		}
	}

	// Try to download fresh rules
	data, err := downloadRules()
	if err != nil {
		// Fall back to stale cache if available
		if cached, err := os.ReadFile(path); err == nil {
			return string(cached)
		}
		return ""
	}

	// Cache the downloaded rules
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, data, 0644)

	return string(data)
}

func downloadRules() ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(rulesURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func parseGitleaksPatterns(config string) []*regexp.Regexp {
	var result []*regexp.Regexp
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "regex") {
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
		} else if strings.HasPrefix(val, `"`) {
			raw = unquoteTOML(val)
		} else {
			continue
		}

		if raw == "" {
			continue
		}

		re, err := regexp.Compile(raw)
		if err != nil {
			continue
		}
		result = append(result, re)
	}
	return result
}

func unquoteTOML(s string) string {
	if len(s) < 2 {
		return s
	}
	if s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
		s = strings.ReplaceAll(s, `\"`, `"`)
		s = strings.ReplaceAll(s, `\\`, `\`)
	}
	return s
}

func Text(s string) string {
	loadPatterns()
	for _, p := range patterns {
		s = p.ReplaceAllString(s, redacted)
	}
	return s
}

func SensitivePath(path string) bool {
	lower := strings.ToLower(path)
	for _, p := range []string{".ssh/", ".env", "/etc/shadow", "/etc/passwd", ".aws/credentials", ".netrc"} {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func PatternCount() int {
	loadPatterns()
	return len(patterns)
}

func RulesAge() time.Duration {
	info, err := os.Stat(cachePath())
	if err != nil {
		return 0
	}
	return time.Since(info.ModTime())
}

func RefreshRules() error {
	data, err := downloadRules()
	if err != nil {
		return err
	}
	path := cachePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	return os.WriteFile(path, data, 0644)
}
