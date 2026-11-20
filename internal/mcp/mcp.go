// Package mcp is the Ferriage MCP server (CONCEPT: MCP server
// surface; ADR 0010). It translates the peer's typed HTTP API
// into Model Context Protocol tools — a protocol translation,
// never a second implementation. A tools/call dispatches as an
// in-process HTTP request through the bound peer mux, so AI
// agents see exactly what the UI and CLI see (handlers,
// validation, audit middleware, actor header).
//
// Transport: MCP Streamable HTTP, hand-rolled per the console
// precedent (ADR 0005): JSON-RPC 2.0 over POST /mcp, no SDK
// dependency, stateless (no session id) — every request carries
// its own tool name and arguments. GET /mcp describes the
// surface; DELETE /mcp is the stateless close.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/NopeNix/Ferriage/internal/audit"
	"github.com/NopeNix/Ferriage/internal/version"
)

const (
	// ProtocolVersion is the MCP spec version we negotiate. The
	// server accepts any client version header and replies with
	// this value (ADR 0010).
	ProtocolVersion = "2025-03-26"
	// ServerName is the MCP server identity.
	ServerName = "ferriage"
)

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Register serves the MCP surface on the peer mux. peer is the
// same mux cmd/peer builds for REST; tool calls are replayed
// through it. Register it after every other route so the tool
// table always resolves against the final mux. auditStore
// records the inner request of each mutating tool call (the
// MCP envelope itself is mesh machinery — see audit.Record);
// nil disables recording (tests).
func Register(mux *http.ServeMux, peer http.Handler, auditStore *audit.Store) {
	h := &Handler{Peer: peer, Audit: auditStore}
	mux.HandleFunc("POST /mcp", h.servePost)
	mux.HandleFunc("GET /mcp", h.serveGet)
	mux.HandleFunc("DELETE /mcp", h.serveDelete)
}

// Handler serves /mcp.
type Handler struct {
	// Peer is the bound peer mux. Nil means the handler cannot
	// dispatch tools (tests may stub it with a mux of their own).
	Peer http.Handler
	// Audit records mutating tool calls as operator actions
	// (deploy, exec, ...). Nil disables recording.
	Audit *audit.Store
}

// serveGet answers with the tool inventory for non-MCP clients
// (curl, browsers, operators checking the surface by hand).
func (h *Handler) serveGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"server":    serverInfo{Name: ServerName, Version: version.Version},
		"protocol":  ProtocolVersion,
		"tools":     toolNames(),
		"transport": "mcp-streamable-http",
		"stateless": true,
		"endpoints": map[string]string{"post": "/mcp", "get": "/mcp", "delete": "/mcp"},
		"doc":       "docs/decisions/0010-mcp-server.md",
	})
}

func (h *Handler) serveDelete(w http.ResponseWriter, r *http.Request) {
	// Stateless: nothing to tear down, but acknowledge the
	// close so MCP clients that send it do not stall.
	w.WriteHeader(http.StatusAccepted)
}

// jsonRPC is one inbound MCP request.
type jsonRPC struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// rpcResult is one outbound MCP response. Exactly one of Result
// or Error is set (except notifications, which are answered
// with an empty 202 body).
type rpcResult struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	// Result is emitted as JSON when set; nil with OmitEmpty via
	// raw form below (Result may be any JSON value).
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// idValue extracts the JSON-RPC id (string or number) for the
// response envelope. A request without an id is a notification.
func (req *jsonRPC) idValue() any {
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(req.ID, &s); err == nil {
		return s
	}
	var n float64
	if err := json.Unmarshal(req.ID, &n); err == nil {
		// JSON numbers decode as float64; an integer id stays
		// an integer in the response.
		if n == float64(int64(n)) {
			return int64(n)
		}
		return n
	}
	return nil
}

func (h *Handler) servePost(w http.ResponseWriter, r *http.Request) {
	// Cap the body: MCP payloads are tool arguments, not
	// snapshots (import takes base64, itself bounded).
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		writeRPCError(w, nil, -32700, "parse error: body too large or unreadable")
		return
	}
	// Batch requests (an array) are answered with an array of
	// responses, per JSON-RPC 2.0. Each element is handled
	// independently; one bad element never fails the batch.
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(trimmed, &batch); err != nil {
			writeRPCError(w, nil, -32700, "parse error: "+err.Error())
			return
		}
		var out []rpcResult
		for _, raw := range batch {
			var req jsonRPC
			if err := json.Unmarshal(raw, &req); err != nil {
				out = append(out, rpcResult{JSONRPC: "2.0", ID: nil, Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}})
				continue
			}
			out = append(out, h.handle(r, &req))
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		_ = enc.Encode(out)
		return
	}
	var req jsonRPC
	if err := json.Unmarshal(trimmed, &req); err != nil {
		writeRPCError(w, nil, -32700, "parse error: "+err.Error())
		return
	}
	// Notifications (no id) get an empty 202 — there is
	// nothing to answer with, and MCP clients expect it.
	if len(req.ID) == 0 || string(req.ID) == "null" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	resp := h.handle(r, &req)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handle executes one JSON-RPC request and returns its response.
func (h *Handler) handle(r *http.Request, req *jsonRPC) rpcResult {
	id := req.idValue()
	switch req.Method {
	case "initialize":
		return rpcResult{JSONRPC: "2.0", ID: id, Result: mustJSON(map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      serverInfo{Name: ServerName, Version: version.Version},
		})}
	case "notifications/initialized", "notifications/cancelled":
		// Informational — answered as a request anyway so the
		// caller gets a well-formed envelope.
		return rpcResult{JSONRPC: "2.0", ID: id, Result: mustJSON(map[string]any{})}
	case "ping":
		return rpcResult{JSONRPC: "2.0", ID: id, Result: mustJSON(map[string]any{})}
	case "tools/list":
		return rpcResult{JSONRPC: "2.0", ID: id, Result: mustJSON(map[string]any{"tools": toolList()})}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return rpcResult{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}}
		}
		res, err := h.callTool(r.Context(), r, params.Name, params.Arguments)
		if err != nil {
			return rpcResult{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32602, Message: err.Error()}}
		}
		return rpcResult{JSONRPC: "2.0", ID: id, Result: mustJSON(res)}
	case "resources/list", "prompts/list":
		// Valid MCP surface with nothing Ferriage serves:
		// an empty list is the correct answer, not an error.
		key := "resources"
		if req.Method == "prompts/list" {
			key = "prompts"
		}
		return rpcResult{JSONRPC: "2.0", ID: id, Result: mustJSON(map[string]any{key: []any{}})}
	default:
		return rpcResult{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}}
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func writeRPCError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResult{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

// callTool dispatches one tool call to the peer mux and
// packages the HTTP response as an MCP tool result. Peer
// errors (HTTP 4xx/5xx) become tool errors (isError), never
// JSON-RPC protocol errors — the call itself succeeded, the
// operation failed.
func (h *Handler) callTool(ctx context.Context, r *http.Request, name string, args json.RawMessage) (map[string]any, error) {
	tool := findTool(name)
	if tool == nil {
		return nil, fmt.Errorf("unknown tool %q (see tools/list for %d tools)", name, len(tools))
	}
	if h.Peer == nil {
		return nil, fmt.Errorf("mcp: no peer mux bound")
	}
	parsed, err := parseArgs(args)
	if err != nil {
		return nil, err
	}
	method, path, body, err := tool.build(parsed)
	if err != nil {
		return nil, err
	}
	// Replay through the peer mux. The actor header is the
	// caller's (BFF-stamped) or "mcp" for direct traffic —
	// an AI client IS a trusted operator (Zone 2, ADR 0010).
	in := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	if actor := r.Header.Get("X-Ferriage-Actor"); actor != "" {
		in.Header.Set("X-Ferriage-Actor", actor)
	} else {
		in.Header.Set("X-Ferriage-Actor", "mcp")
	}
	if len(body) > 0 {
		in.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.Peer.ServeHTTP(rec, in)
	// The inner request is the audited action (deploy, exec,
	// update.run, ...), not the MCP envelope: record it the
	// same way the middleware records browser/CLI traffic.
	// Reads (GET) classify to unaudited and are skipped.
	if h.Audit != nil {
		audit.Record(h.Audit, in, rec.Code)
	}
	return toolResult(rec.Code, rec.Body.Bytes()), nil
}

// toolResult packages an HTTP response as an MCP CallToolResult:
// text content is the peer's JSON response verbatim (agents
// parse it directly), with isError set on non-2xx. Large
// bodies stay verbatim — truncation would hide the operation
// outcome from the agent.
func toolResult(code int, body []byte) map[string]any {
	isErr := code < 200 || code >= 300
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = fmt.Sprintf("peer returned HTTP %d", code)
	}
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
		"isError": isErr,
	}
}

// toolNames returns the tool names in table order (deterministic
// for GET /mcp and tests).
func toolNames() []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

// numberArg extracts an integer tool argument with a default.
func numberArg(args map[string]any, key string, def int) (int, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return def, nil
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case string:
		if n == "" {
			return def, nil
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, fmt.Errorf("argument %q must be an integer", key)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("argument %q must be an integer", key)
	}
}

// stringArg extracts a string tool argument (empty if absent).
func stringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// parseArgs decodes tool arguments into a map, tolerating an
// empty payload (tools with no arguments).
func parseArgs(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if out == nil {
		return map[string]any{}, nil
	}
	return out, nil
}

// intQuery formats an optional integer query parameter.
func intQuery(key string, n int) string {
	if n <= 0 {
		return ""
	}
	return key + "=" + strconv.Itoa(n)
}

// jsonBody marshals a map for a peer request body, or returns
// nil when the map is empty (GET-style calls carry no body).
func jsonBody(m map[string]any) []byte {
	if len(m) == 0 {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

// encodeQuery joins non-empty fragments.
func encodeQuery(frags ...string) string {
	var out []string
	for _, f := range frags {
		if f != "" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return ""
	}
	return "?" + strings.Join(out, "&")
}
