package sanitize

import (
	_ "embed"
	"regexp"
	"strings"
	"sync"
)

//go:embed gitleaks.toml
var gitleaksConfig string

var (
	patternsOnce sync.Once
	patterns     []*regexp.Regexp
)

func loadPatterns() {
	patternsOnce.Do(func() {
		// Parse regex patterns from gitleaks TOML
		// Each rule has: regex = '''...''' or regex = "..."
		for _, line := range strings.Split(gitleaksConfig, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "regex") {
				continue
			}
			idx := strings.Index(line, "=")
			if idx < 0 {
				continue
			}
			val := strings.TrimSpace(line[idx+1:])

			// Extract the regex string from TOML quoting
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

			// Go's regexp doesn't support some Perl features — skip those
			re, err := regexp.Compile(raw)
			if err != nil {
				continue
			}
			patterns = append(patterns, re)
		}

		// Additional patterns not in gitleaks
		extra := []string{
			`(?i)(-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----)`,
			`(?i)export\s+\w+=\S{8,}`,
		}
		for _, e := range extra {
			if re, err := regexp.Compile(e); err == nil {
				patterns = append(patterns, re)
			}
		}
	})
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

const redacted = "[REDACTED]"

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
