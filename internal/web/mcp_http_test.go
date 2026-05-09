package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"vibecockpit/internal/config"
	mcpserver "vibecockpit/internal/mcp"
)

// TestMCPHTTP_ToolsListRoundTrip wires up just enough of the web server
// to exercise /mcp end-to-end: mux + handler + the real mcp.Server. We
// don't go through Start() because the full Start() sets up scheduler,
// chat, and a memory index — none of which the MCP transport actually
// needs.
func TestMCPHTTP_ToolsListRoundTrip(t *testing.T) {
	cfg := &config.Config{EnableMCP: true}
	mcp := mcpserver.NewServer(nil, "test", "/tmp", cfg, nil)

	s := &server{mcpServer: mcp}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", s.handleMCPHTTP)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	resp, err := http.Post(ts.URL+"/mcp", "application/json", bytes.NewReader(req))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
		ID int `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n  body: %s", err, string(body))
	}
	if parsed.ID != 1 {
		t.Errorf("response id=%d; want 1 (must echo request id)", parsed.ID)
	}
	if len(parsed.Result.Tools) == 0 {
		t.Fatal("expected at least one tool in tools/list response")
	}
	// Sanity: the search_memory tool we added in v0.14.0 must be in the list.
	var found bool
	for _, tool := range parsed.Result.Tools {
		if name, _ := tool["name"].(string); name == "search_memory" {
			found = true
			break
		}
	}
	if !found {
		t.Error("search_memory not in tools/list — toolset wiring is broken")
	}
}

func TestMCPHTTP_DisabledReturns503(t *testing.T) {
	s := &server{mcpServer: nil} // MCP disabled in config
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", s.handleMCPHTTP)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/mcp", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("status=%d; want 503 when mcpServer is nil", resp.StatusCode)
	}
}

func TestMCPHTTP_NotificationReturns204(t *testing.T) {
	cfg := &config.Config{EnableMCP: true}
	mcp := mcpserver.NewServer(nil, "test", "/tmp", cfg, nil)

	s := &server{mcpServer: mcp}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", s.handleMCPHTTP)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// notifications/initialized has no id → MCP server writes nothing →
	// HTTP layer should reply 204.
	req := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	resp, err := http.Post(ts.URL+"/mcp", "application/json", bytes.NewReader(req))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("status=%d body=%q; want 204", resp.StatusCode, string(body))
	}
}
