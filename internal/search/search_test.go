package search

import "testing"

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		text      string
		wantMatch bool
		wantMin   int // minimum expected score (0 = don't check)
	}{
		{"exact substring", "ello", "Hello World", true, 100},
		{"prefix bonus", "hel", "hello", true, 150},
		{"separator bonus", "world", "hello-world", true, 130},
		{"fuzzy char-by-char", "hw", "Hello World", true, 1},
		{"no match", "xyz", "hello", false, 0},
		{"empty pattern", "", "anything", true, 0},
		{"pattern longer than text", "longpattern", "short", false, 0},
		{"case insensitive", "HELLO", "hello world", true, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, score := FuzzyMatch(tt.pattern, tt.text)
			if ok != tt.wantMatch {
				t.Fatalf("FuzzyMatch(%q, %q) match = %v, want %v", tt.pattern, tt.text, ok, tt.wantMatch)
			}
			if ok && tt.wantMin > 0 && score < tt.wantMin {
				t.Errorf("FuzzyMatch(%q, %q) score = %d, want >= %d", tt.pattern, tt.text, score, tt.wantMin)
			}
		})
	}
}

func TestFuzzyMatchMulti(t *testing.T) {
	t.Run("all terms match", func(t *testing.T) {
		ok, score := FuzzyMatchMulti([]string{"hel", "wor"}, "hello", "world")
		if !ok || score == 0 {
			t.Fatalf("expected match with positive score, got ok=%v score=%d", ok, score)
		}
	})

	t.Run("one term fails", func(t *testing.T) {
		ok, _ := FuzzyMatchMulti([]string{"hel", "xyz"}, "hello", "world")
		if ok {
			t.Fatal("expected no match when one term fails")
		}
	})

	t.Run("empty patterns", func(t *testing.T) {
		ok, score := FuzzyMatchMulti([]string{}, "hello")
		if !ok || score != 0 {
			t.Fatalf("empty patterns: got ok=%v score=%d, want ok=true score=0", ok, score)
		}
	})
}

func TestParseQuery(t *testing.T) {
	t.Run("plain terms", func(t *testing.T) {
		q := ParseQuery("hello world")
		if len(q.FuzzyTerms) != 2 || q.FuzzyTerms[0] != "hello" || q.FuzzyTerms[1] != "world" {
			t.Fatalf("unexpected FuzzyTerms: %v", q.FuzzyTerms)
		}
	})

	t.Run("model filter", func(t *testing.T) {
		q := ParseQuery("model:opus branch:main")
		if q.Filters["model"] != "opus" {
			t.Errorf("model filter = %q, want %q", q.Filters["model"], "opus")
		}
		if q.Filters["branch"] != "main" {
			t.Errorf("branch filter = %q, want %q", q.Filters["branch"], "main")
		}
	})

	t.Run("active keyword", func(t *testing.T) {
		q := ParseQuery("active foo")
		if !q.ActiveOnly {
			t.Error("expected ActiveOnly = true")
		}
		if len(q.FuzzyTerms) != 1 || q.FuzzyTerms[0] != "foo" {
			t.Errorf("unexpected FuzzyTerms: %v", q.FuzzyTerms)
		}
	})

	t.Run("mixed input", func(t *testing.T) {
		q := ParseQuery("active model:opus myterm")
		if !q.ActiveOnly {
			t.Error("expected ActiveOnly")
		}
		if q.Filters["model"] != "opus" {
			t.Error("expected model filter")
		}
		if len(q.FuzzyTerms) != 1 || q.FuzzyTerms[0] != "myterm" {
			t.Errorf("unexpected FuzzyTerms: %v", q.FuzzyTerms)
		}
	})

	t.Run("unknown filter key becomes fuzzy term", func(t *testing.T) {
		q := ParseQuery("foo:bar")
		if len(q.FuzzyTerms) != 1 || q.FuzzyTerms[0] != "foo:bar" {
			t.Errorf("expected unknown key:val as fuzzy term, got %v", q.FuzzyTerms)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		q := ParseQuery("")
		if !q.IsEmpty() {
			t.Error("expected IsEmpty() for empty input")
		}
	})

	t.Run("IsEmpty false when has terms", func(t *testing.T) {
		q := ParseQuery("hello")
		if q.IsEmpty() {
			t.Error("expected IsEmpty() = false")
		}
	})
}

func TestFieldContains(t *testing.T) {
	tests := []struct {
		field, pattern string
		want           bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "WORLD", true},
		{"Hello", "xyz", false},
		{"", "", true},
	}
	for _, tt := range tests {
		if got := FieldContains(tt.field, tt.pattern); got != tt.want {
			t.Errorf("FieldContains(%q, %q) = %v, want %v", tt.field, tt.pattern, got, tt.want)
		}
	}
}
