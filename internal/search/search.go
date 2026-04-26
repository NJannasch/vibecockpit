package search

import (
	"strings"
	"unicode"
)

type Query struct {
	FuzzyTerms []string
	Filters    map[string]string
	ActiveOnly bool
}

func ParseQuery(input string) Query {
	q := Query{Filters: make(map[string]string)}
	parts := strings.Fields(input)

	for _, p := range parts {
		lower := strings.ToLower(p)
		if lower == "active" {
			q.ActiveOnly = true
			continue
		}
		if idx := strings.IndexByte(p, ':'); idx > 0 && idx < len(p)-1 {
			key := strings.ToLower(p[:idx])
			val := strings.ToLower(p[idx+1:])
			switch key {
			case "model", "branch", "project", "provider":
				q.Filters[key] = val
				continue
			}
		}
		q.FuzzyTerms = append(q.FuzzyTerms, lower)
	}
	return q
}

func (q Query) IsEmpty() bool {
	return len(q.FuzzyTerms) == 0 && len(q.Filters) == 0 && !q.ActiveOnly
}

func FuzzyMatch(pattern, text string) (bool, int) {
	pattern = strings.ToLower(pattern)
	text = strings.ToLower(text)

	if pattern == "" {
		return true, 0
	}
	if len(pattern) > len(text) {
		return false, 0
	}

	if idx := strings.Index(text, pattern); idx >= 0 {
		score := 100 + len(pattern)*3
		if idx == 0 {
			score += 50
		}
		if idx > 0 && isSep(rune(text[idx-1])) {
			score += 30
		}
		return true, score
	}

	rp := []rune(pattern)
	rt := []rune(text)
	pi := 0
	score := 0
	consec := 0

	for ti := 0; ti < len(rt) && pi < len(rp); ti++ {
		if unicode.ToLower(rt[ti]) == unicode.ToLower(rp[pi]) {
			score++
			consec++
			if consec > 1 {
				score += consec
			}
			if ti == 0 || isSep(rt[ti-1]) {
				score += 5
			}
			if ti > 0 && unicode.IsUpper(rt[ti]) {
				score += 3
			}
			pi++
		} else {
			consec = 0
		}
	}

	if pi < len(rp) {
		return false, 0
	}
	return true, score
}

func FuzzyMatchMulti(patterns []string, fields ...string) (bool, int) {
	totalScore := 0
	for _, p := range patterns {
		bestScore := 0
		matched := false
		for _, f := range fields {
			if ok, s := FuzzyMatch(p, f); ok && s > bestScore {
				bestScore = s
				matched = true
			}
		}
		if !matched {
			return false, 0
		}
		totalScore += bestScore
	}
	return true, totalScore
}

func FieldContains(field, pattern string) bool {
	return strings.Contains(strings.ToLower(field), strings.ToLower(pattern))
}

func isSep(r rune) bool {
	return r == ' ' || r == '-' || r == '_' || r == '/' || r == '.' || r == '\\'
}
