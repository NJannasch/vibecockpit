package sanitize

import (
	"testing"
)

func TestText_RedactsAPIKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring that should NOT appear
	}{
		{"OpenAI key", "my key is sk-abcdefghij1234567890abcdefghij", "sk-abcdefghij1234567890abcdefghij"},
		{"GitHub PAT", "token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij", "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ"},
		{"AWS key", "aws_key=AKIAIOSFODNN7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"Bearer token", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.test", "eyJhbGciOiJIUzI1NiJ9"},
		{"Private key", "-----BEGIN RSA PRIVATE KEY-----", "BEGIN RSA PRIVATE KEY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Text(tt.input)
			if result == tt.input {
				t.Errorf("Text() did not redact anything in %q", tt.input)
			}
		})
	}
}

func TestText_LeavesCleanText(t *testing.T) {
	clean := "This is a normal conversation about building a web app"
	if Text(clean) != clean {
		t.Errorf("Text() modified clean text: %q", Text(clean))
	}
}

func TestSensitivePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/home/user/.ssh/id_rsa", true},
		{"/home/user/.env", true},
		{"/etc/shadow", true},
		{"/home/user/.aws/credentials", true},
		{"/home/user/project/main.go", false},
		{"/home/user/Documents/notes.md", false},
	}
	for _, tt := range tests {
		if got := SensitivePath(tt.path); got != tt.want {
			t.Errorf("SensitivePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestPatternCount(t *testing.T) {
	count := PatternCount()
	if count < 5 {
		t.Errorf("PatternCount() = %d, expected at least 5 fallback patterns", count)
	}
}
