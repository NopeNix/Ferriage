package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePeer is a miniature peer mux standing in for the real
// one: it must record what the tool dispatch sent (method,
// path, actor) and answer with canned HTTP responses, so the
// tests prove "tool call = in-process request" without Docker.
func fakePeer(t *testing.T) (*http.ServeMux, *[]string) {
	t.Helper()
	var calls []string
	mux := http.NewServeMux()
	record := func(label string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			*callsPtr = append(*callsPtr, label+" "+r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" actor="+r.Header.Get("X-Ferriage-Actor"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}
	mux.HandleFunc("GET /stacks", record("stacks"))
	mux.HandleFunc("GET /peer", record("peer"))
	mux.HandleFunc("POST /stacks/{name}/deploy", record("deploy"))
	mux.HandleFunc("GET /audit", record("audit"))
	mux.HandleFunc("POST /update/{name}/check", record("check"))
	return mux, &calls
}

var callsPtr *[]string

func postMCP(t *testing.T, h *Handler, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.servePost(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func TestInitialize(t *testing.T) {
	mux, _ := fakePeer(t)
	callsPtr = nil
	h := &Handler{Peer: mux}
	code, out := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if code != http.StatusOK {
		t.Fatalf("code = %d", code)
	}
	result, ok := out["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", out)
	}
	if result["protocolVersion"] != ProtocolVersion {
		t.Errorf("protocolVersion = %v, want %s", result["protocolVersion"], ProtocolVersion)
	}
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != ServerName {
		t.Errorf("serverInfo.name = %v", info["name"])
	}
}

func TestToolsList(t *testing.T) {
	mux, _ := fakePeer(t)
	h := &Handler{Peer: mux}
	_, out := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	result := out["result"].(map[string]any)
	list, ok := result["tools"].([]any)
	if !ok || len(list) != len(tools) {
		t.Fatalf("tools/list = %v (len %d, want %d)", result["tools"], len(list), len(tools))
	}
	names := map[string]bool{}
	for _, entry := range list {
		m := entry.(map[string]any)
		names[m["name"].(string)] = true
		if _, ok := m["inputSchema"]; !ok {
			t.Errorf("tool %v missing inputSchema", m["name"])
		}
		if _, ok := m["annotations"]; !ok {
			t.Errorf("tool %v missing annotations", m["name"])
		}
	}
	if !names["list_stacks"] || !names["deploy_stack"] {
		t.Errorf("expected tools missing: %v", names)
	}
	// Every tool in the table appears exactly once.
	for _, tool := range tools {
		if !names[tool.Name] {
			t.Errorf("table tool %q missing from tools/list", tool.Name)
		}
	}
}

func TestToolsCallListStacks(t *testing.T) {
	mux, calls := fakePeer(t)
	callsPtr = calls
	h := &Handler{Peer: mux}
	_, out := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_stacks","arguments":{}}}`)
	result := out["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("isError = %v", result["isError"])
	}
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"ok":true`) {
		t.Errorf("text = %q", text)
	}
	if len(*calls) != 1 || !strings.HasPrefix((*calls)[0], "stacks GET /stacks") {
		t.Errorf("calls = %v", *calls)
	}
	if !strings.Contains((*calls)[0], "actor=mcp") {
		t.Errorf("actor = %v, want mcp", *calls)
	}
}

func TestToolsCallPassesActor(t *testing.T) {
	mux, calls := fakePeer(t)
	callsPtr = calls
	h := &Handler{Peer: mux}
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_peer","arguments":{}}}`))
	req.Header.Set("X-Ferriage-Actor", "alice")
	rec := httptest.NewRecorder()
	h.servePost(rec, req)
	if len(*calls) != 1 || !strings.Contains((*calls)[0], "actor=alice") {
		t.Errorf("calls = %v", *calls)
	}
}

func TestToolsCallDeployBuildsPath(t *testing.T) {
	mux, calls := fakePeer(t)
	callsPtr = calls
	h := &Handler{Peer: mux}
	_, out := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"deploy_stack","arguments":{"stack":"demo"}}}`)
	if out["result"].(map[string]any)["isError"] != false {
		t.Fatalf("isError: %v", out)
	}
	if len(*calls) != 1 || !strings.Contains((*calls)[0], "deploy POST /stacks/demo/deploy") {
		t.Errorf("calls = %v", *calls)
	}
}

func TestToolsCallUnknownTool(t *testing.T) {
	mux, _ := fakePeer(t)
	h := &Handler{Peer: mux}
	_, out := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	errObj, ok := out["error"].(map[string]any)
	if !ok || errObj["code"].(float64) != -32602 {
		t.Fatalf("error = %v", out["error"])
	}
	if !strings.Contains(errObj["message"].(string), "unknown tool") {
		t.Errorf("message = %v", errObj["message"])
	}
}

func TestUnknownMethod(t *testing.T) {
	mux, _ := fakePeer(t)
	h := &Handler{Peer: mux}
	_, out := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"bogus/method"}`)
	errObj := out["error"].(map[string]any)
	if errObj["code"].(float64) != -32601 {
		t.Errorf("code = %v", errObj["code"])
	}
}

func TestNotificationGets202(t *testing.T) {
	mux, _ := fakePeer(t)
	h := &Handler{Peer: mux}
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	rec := httptest.NewRecorder()
	h.servePost(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Errorf("code = %d, want 202", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
}

func TestBatch(t *testing.T) {
	mux, _ := fakePeer(t)
	h := &Handler{Peer: mux}
	body := `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","id":2,"method":"tools/list"}]`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.servePost(rec, req)
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("batch response: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("batch len = %d", len(out))
	}
}

func TestGetAndDelete(t *testing.T) {
	mux, _ := fakePeer(t)
	h := &Handler{Peer: mux}
	rec := httptest.NewRecorder()
	h.serveGet(rec, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET code = %d", rec.Code)
	}
	var desc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &desc); err != nil {
		t.Fatalf("GET body: %v", err)
	}
	if desc["transport"] != "mcp-streamable-http" {
		t.Errorf("transport = %v", desc["transport"])
	}
	rec2 := httptest.NewRecorder()
	h.serveDelete(rec2, httptest.NewRequest(http.MethodDelete, "/mcp", nil))
	if rec2.Code != http.StatusAccepted {
		t.Errorf("DELETE code = %d", rec2.Code)
	}
}

// Tool builders get a direct test too: argument validation
// must produce tool errors, not peer 400s.
func TestToolBuilders(t *testing.T) {
	mux, _ := fakePeer(t)
	h := &Handler{Peer: mux}
	// Missing stack -> invalid params (no dispatch).
	_, out := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"deploy_stack","arguments":{}}}`)
	errObj, ok := out["error"].(map[string]any)
	if !ok || errObj["code"].(float64) != -32602 {
		t.Fatalf("missing-arg error = %v", out["error"])
	}
	// Audit with limit passes through as a query string.
	_, out2 := postMCP(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_audit","arguments":{"limit":5,"actor":"mcp"}}}`)
	if out2["result"].(map[string]any)["isError"] != false {
		t.Fatalf("audit isError: %v", out2)
	}
}

func TestImportBuildsDecodedBody(t *testing.T) {
	tool := findTool("import_stack")
	if tool == nil {
		t.Fatal("import_stack not found")
	}
	// "aGVsbG8=" is base64 for "hello".
	method, path, body, err := tool.build(map[string]any{"stack": "demo", "archive_b64": "aGVsbG8="})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if method != "POST" || path != "/stacks/demo/import" || string(body) != "hello" {
		t.Errorf("got %s %s %q", method, path, body)
	}
	if _, _, _, err := tool.build(map[string]any{"stack": "demo", "archive_b64": "!!!not-base64"}); err == nil {
		t.Errorf("bad base64 accepted")
	}
}

func TestToolResultWrapsBody(t *testing.T) {
	res := toolResult(200, []byte(`{"stacks":[]}`))
	if res["isError"] != false {
		t.Errorf("200 isError")
	}
	res = toolResult(404, []byte(`{"error":"stack not found"}`))
	if res["isError"] != true {
		t.Errorf("404 not isError")
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "stack not found") {
		t.Errorf("text = %q", text)
	}
}

func TestProtocolVersionAcceptsClientHeader(t *testing.T) {
	// The server negotiates its own version regardless of
	// what the client asks for (stateless, ADR 0010).
	mux, _ := fakePeer(t)
	h := &Handler{Peer: mux}
	_, out := postMCP(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"old","version":"1"}}}`)
	if out["result"].(map[string]any)["protocolVersion"] != ProtocolVersion {
		t.Errorf("protocolVersion = %v", out["result"])
	}
}
