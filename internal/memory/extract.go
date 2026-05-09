package memory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vibecockpit/internal/sanitize"
)

// MaxMessageBytes caps how much text we keep per message. Long
// agent runs can produce hundreds of KB of single-message content;
// the marginal search value past 64 KB is near zero.
const MaxMessageBytes = 64 * 1024

// MaxMessagesPerSession caps total rows per session so a runaway
// transcript doesn't blow up the index.
const MaxMessagesPerSession = 5000

// Message is one transcript entry. Index.PutSession writes one FTS row
// per Message; Index.Context fetches a ±N window around a hit.
type Message struct {
	Idx       int       // 0-based ordinal within the session
	Role      string    // "user" | "assistant"
	Timestamp time.Time // zero when not present in the source
	Content   string    // already-sanitized text
}

// ExtractMessages reads a session's transcript and returns it as a
// list of Messages. Tool-use and image blocks are skipped; secrets
// are scrubbed via internal/sanitize before the text reaches the index.
//
// For unknown formats ExtractMessages returns nil — callers should
// fall back to indexing a synthesized "summary + first prompt"
// pseudo-message rather than treating an empty result as an error.
func ExtractMessages(dataPath string) ([]Message, error) {
	if dataPath == "" {
		return nil, nil
	}
	fi, err := os.Stat(dataPath)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, nil
	}

	switch strings.ToLower(filepath.Ext(dataPath)) {
	case ".jsonl":
		return extractJSONLMessages(dataPath)
	case ".json":
		return extractClaudeDesktopWrapperMessages(dataPath)
	}
	return nil, nil
}

// extractJSONLMessages handles the Claude Code transcript format. Each
// line is a JSON object with `type` and `message.content` (string or
// content blocks).
func extractJSONLMessages(path string) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var msgs []Message
	idx := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)

	for scanner.Scan() {
		if len(msgs) >= MaxMessagesPerSession {
			break
		}
		line := scanner.Bytes()
		if !bytes.Contains(line, []byte(`"message"`)) {
			continue
		}
		var entry struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				Content json.RawMessage `json:"content"`
				Role    string          `json:"role"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}
		text := decodeContent(entry.Message.Content)
		if text == "" {
			continue
		}
		if len(text) > MaxMessageBytes {
			text = text[:MaxMessageBytes]
		}
		text = sanitize.Text(text)
		ts, _ := time.Parse(time.RFC3339, entry.Timestamp)
		msgs = append(msgs, Message{
			Idx:       idx,
			Role:      entry.Type,
			Timestamp: ts,
			Content:   text,
		})
		idx++
	}
	return msgs, nil
}

// decodeContent handles both shapes Claude uses for message.content:
//   - a bare string ("hello there")
//   - an array of content blocks ([{type:"text", text:"..."}, ...])
// Tool-use, image, and other non-text blocks are skipped.
func decodeContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var b strings.Builder
		for _, blk := range blocks {
			if blk.Type == "text" && blk.Text != "" {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(blk.Text)
			}
		}
		return b.String()
	}
	return ""
}

// extractClaudeDesktopWrapperMessages reads a wrapper.json, finds the
// cliSessionId, then extracts messages from the matching JSONL.
func extractClaudeDesktopWrapperMessages(path string) ([]Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var w struct {
		CliSessionID string `json:"cliSessionId"`
		Cwd          string `json:"cwd"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, nil // not a wrapper we recognize; not an error
	}
	if w.CliSessionID == "" {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	projectDir := filepath.Join(home, ".claude", "projects", slugify(w.Cwd))
	jsonl := filepath.Join(projectDir, w.CliSessionID+".jsonl")
	if _, err := os.Stat(jsonl); err != nil {
		matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", w.CliSessionID+".jsonl"))
		if len(matches) == 0 {
			return nil, nil
		}
		jsonl = matches[0]
	}
	return extractJSONLMessages(jsonl)
}

func slugify(cwd string) string {
	if cwd == "" {
		return ""
	}
	return strings.ReplaceAll(cwd, "/", "-")
}

// SessionContent is what the indexer feeds to Index.PutSession. It's
// just enough provider-agnostic metadata that the FTS rows know what
// session they belong to.
type SessionContent struct {
	ID          string
	Provider    string
	ProjectName string
	ProjectPath string
	Model       string
	GitBranch   string
	Modified    time.Time
	Summary     string
	FirstPrompt string
	DataPath    string
}

// BuildSessionDoc walks a session and produces a SessionDoc ready for
// Index.PutSession. If extraction returns no messages we synthesize a
// single pseudo-message from summary+firstPrompt so the session is
// still findable by metadata terms.
func BuildSessionDoc(sc SessionContent) (SessionDoc, error) {
	msgs, err := ExtractMessages(sc.DataPath)
	if err != nil {
		return SessionDoc{}, err
	}
	if len(msgs) == 0 {
		fallback := strings.TrimSpace(sc.Summary + "\n" + sc.FirstPrompt)
		if fallback != "" {
			msgs = []Message{{
				Idx:     0,
				Role:    "user",
				Content: sanitize.Text(fallback),
			}}
		}
	}
	return SessionDoc{
		ID:          sc.ID,
		Provider:    sc.Provider,
		ProjectName: sc.ProjectName,
		ProjectPath: sc.ProjectPath,
		Model:       sc.Model,
		GitBranch:   sc.GitBranch,
		Modified:    sc.Modified,
		Summary:     sanitize.Text(sc.Summary),
		Messages:    msgs,
	}, nil
}
