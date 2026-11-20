package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
)

// tool is one MCP tool: the public shape (name, description,
// input schema) plus how to translate its arguments into a
// peer HTTP request. tools/list is generated from this table,
// so the public list and the dispatcher can never drift
// (ADR 0010).
type tool struct {
	Name        string
	Description string
	// ReadOnly marks tools that never mutate state (MCP
	// readOnlyHint). Destructive marks tools with
	// deploy/exec-grade blast radius (MCP destructiveHint).
	ReadOnly    bool
	Destructive bool
	// Properties is the JSON Schema properties object for
	// the tool's arguments (emitted verbatim in tools/list).
	Properties map[string]any
	// Required lists required argument names.
	Required []string
	// build turns decoded arguments into (method, path,
	// body) for the peer mux. Path includes any query
	// string. Body is nil for bodyless calls.
	build func(args map[string]any) (method, path string, body []byte, err error)
}

// tools is the curated tool table (ADR 0010): everything an
// agent needs to operate the homelab, minus interactive WS
// console, /sync/* mesh wire, and /notify env secrets.
// Table order is the tools/list order.
var tools = []tool{
	{
		Name:        "list_stacks",
		Description: "List adopted Compose stacks on this peer: name, services, running counts. The first call before any other operation.",
		ReadOnly:    true,
		Properties:  map[string]any{},
		Required:    nil,
		build: func(args map[string]any) (string, string, []byte, error) {
			return "GET", "/stacks", nil, nil
		},
	},
	{
		Name:        "get_peer",
		Description: "Peer identity: id, version, commit. Confirms which node an agent is driving (peers are not clients).",
		ReadOnly:    true,
		Properties:  map[string]any{},
		Required:    nil,
		build: func(args map[string]any) (string, string, []byte, error) {
			return "GET", "/peer", nil, nil
		},
	},
	{
		Name:        "deploy_stack",
		Description: "Deploy a stack: brings its Compose file into a healthy state (health-gated). Returns per-service health and an overall healthy flag.",
		ReadOnly:    false,
		Destructive: true,
		Properties: map[string]any{
			"stack": map[string]any{"type": "string", "description": "Stack name (state-directory name)"},
		},
		Required: []string{"stack"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack := stringArg(args, "stack")
			if stack == "" {
				return "", "", nil, fmt.Errorf("deploy_stack requires argument \"stack\"")
			}
			return "POST", "/stacks/" + url.PathEscape(stack) + "/deploy", nil, nil
		},
	},
	{
		Name:        "export_stack",
		Description: "Snapshot a stack with its volumes to one gzip+tar (ADR 0002). Returns the archive as base64; decode it to a file to move the stack to another peer.",
		ReadOnly:    true,
		Properties: map[string]any{
			"stack": map[string]any{"type": "string", "description": "Stack name"},
		},
		Required: []string{"stack"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack := stringArg(args, "stack")
			if stack == "" {
				return "", "", nil, fmt.Errorf("export_stack requires argument \"stack\"")
			}
			return "POST", "/stacks/" + url.PathEscape(stack) + "/export", nil, nil
		},
	},
	{
		Name:        "import_stack",
		Description: "Restore a snapshot archive into a fresh stack directory and volumes. The archive is base64-encoded gzip+tar (the same format ferriage export writes). Refuses if the stack already exists.",
		ReadOnly:    false,
		Destructive: true,
		Properties: map[string]any{
			"stack":       map[string]any{"type": "string", "description": "Stack name to restore as"},
			"archive_b64": map[string]any{"type": "string", "description": "Base64-encoded snapshot archive (gzip+tar)"},
		},
		Required: []string{"stack", "archive_b64"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack := stringArg(args, "stack")
			archive := stringArg(args, "archive_b64")
			if stack == "" || archive == "" {
				return "", "", nil, fmt.Errorf("import_stack requires arguments \"stack\" and \"archive_b64\"")
			}
			// Validate it is really base64 here so the agent
			// gets a tool error, not a peer 400.
			if _, err := base64.StdEncoding.DecodeString(archive); err != nil {
				return "", "", nil, fmt.Errorf("import_stack: archive_b64 is not valid base64: %w", err)
			}
			// The peer expects raw gzip bytes; the tool
			// carries base64. Decode here and hand the
			// peer the bytes the REST API would receive.
			raw, _ := base64.StdEncoding.DecodeString(archive)
			return "POST", "/stacks/" + url.PathEscape(stack) + "/import", raw, nil
		},
	},
	{
		Name:        "get_logs",
		Description: "Tail one container's logs (non-follow, bounded). Returns lines with timestamps and streams.",
		ReadOnly:    true,
		Properties: map[string]any{
			"stack":   map[string]any{"type": "string", "description": "Stack name"},
			"service": map[string]any{"type": "string", "description": "Compose service name"},
			"tail":    map[string]any{"type": "integer", "description": "Lines to return (1..10000, default 200)"},
			"since":   map[string]any{"type": "string", "description": "RFC 3339 timestamp; only logs after this"},
		},
		Required: []string{"stack", "service"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack, svc := stringArg(args, "stack"), stringArg(args, "service")
			if stack == "" || svc == "" {
				return "", "", nil, fmt.Errorf("get_logs requires arguments \"stack\" and \"service\"")
			}
			tail, err := numberArg(args, "tail", 200)
			if err != nil {
				return "", "", nil, err
			}
			if tail < 1 || tail > 10000 {
				return "", "", nil, fmt.Errorf("get_logs: tail must be 1..10000")
			}
			q := encodeQuery(intQuery("tail", tail), func() string {
				if s := stringArg(args, "since"); s != "" {
					return "since=" + url.QueryEscape(s)
				}
				return ""
			}())
			return "GET", "/stacks/" + url.PathEscape(stack) + "/services/" + url.PathEscape(svc) + "/logs" + q, nil, nil
		},
	},
	{
		Name:        "get_stats",
		Description: "One-shot live CPU/memory/network snapshot for every container of a stack (docker stats, non-streaming).",
		ReadOnly:    true,
		Properties: map[string]any{
			"stack": map[string]any{"type": "string", "description": "Stack name"},
		},
		Required: []string{"stack"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack := stringArg(args, "stack")
			if stack == "" {
				return "", "", nil, fmt.Errorf("get_stats requires argument \"stack\"")
			}
			return "GET", "/stacks/" + url.PathEscape(stack) + "/stats", nil, nil
		},
	},
	{
		Name:        "exec_in_service",
		Description: "Run a one-shot command inside one running container. Non-interactive (the TTY console is a WebSocket, not a tool). Returns exit code and captured output, capped at 1MB.",
		ReadOnly:    false,
		Destructive: true,
		Properties: map[string]any{
			"stack":   map[string]any{"type": "string", "description": "Stack name"},
			"service": map[string]any{"type": "string", "description": "Compose service name"},
			"cmd":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Command argv, e.g. [\"cat\", \"/data/marker.txt\"]"},
		},
		Required: []string{"stack", "service", "cmd"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack, svc := stringArg(args, "stack"), stringArg(args, "service")
			if stack == "" || svc == "" {
				return "", "", nil, fmt.Errorf("exec_in_service requires arguments \"stack\" and \"service\"")
			}
			raw, ok := args["cmd"]
			if !ok {
				return "", "", nil, fmt.Errorf("exec_in_service requires argument \"cmd\"")
			}
			var argv []string
			decoded, err := json.Marshal(raw)
			if err != nil {
				return "", "", nil, fmt.Errorf("exec_in_service: cmd: %w", err)
			}
			if err := json.Unmarshal(decoded, &argv); err != nil || len(argv) == 0 {
				return "", "", nil, fmt.Errorf("exec_in_service: cmd must be a non-empty array of strings")
			}
			body := jsonBody(map[string]any{"cmd": argv})
			return "POST", "/stacks/" + url.PathEscape(stack) + "/services/" + url.PathEscape(svc) + "/exec", body, nil
		},
	},
	{
		Name:        "get_compose",
		Description: "Read a stack's compose.yaml content from the state directory (files are the source of truth).",
		ReadOnly:    true,
		Properties: map[string]any{
			"stack": map[string]any{"type": "string", "description": "Stack name"},
		},
		Required: []string{"stack"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack := stringArg(args, "stack")
			if stack == "" {
				return "", "", nil, fmt.Errorf("get_compose requires argument \"stack\"")
			}
			return "GET", "/stacks/" + url.PathEscape(stack) + "/compose", nil, nil
		},
	},
	{
		Name:        "save_compose",
		Description: "Replace a stack's compose.yaml content atomically (crash-safe via temp file + rename). Does not deploy — call deploy_stack after.",
		ReadOnly:    false,
		Properties: map[string]any{
			"stack":   map[string]any{"type": "string", "description": "Stack name"},
			"content": map[string]any{"type": "string", "description": "New compose.yaml content"},
		},
		Required: []string{"stack", "content"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack := stringArg(args, "stack")
			content := stringArg(args, "content")
			if stack == "" {
				return "", "", nil, fmt.Errorf("save_compose requires argument \"stack\"")
			}
			if _, ok := args["content"]; !ok {
				return "", "", nil, fmt.Errorf("save_compose requires argument \"content\"")
			}
			body := jsonBody(map[string]any{"content": content})
			return "PUT", "/stacks/" + url.PathEscape(stack) + "/compose", body, nil
		},
	},
	{
		Name:        "check_update",
		Description: "Force a synchronous digest check for one stack (registry vs running images). Never deploys — use run_update for the snapshot-first redeploy. Returns the fresh update report.",
		ReadOnly:    true,
		Properties: map[string]any{
			"stack": map[string]any{"type": "string", "description": "Stack name"},
		},
		Required: []string{"stack"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack := stringArg(args, "stack")
			if stack == "" {
				return "", "", nil, fmt.Errorf("check_update requires argument \"stack\"")
			}
			return "POST", "/update/" + url.PathEscape(stack) + "/check", nil, nil
		},
	},
	{
		Name:        "set_update_policy",
		Description: "Set a stack's auto-update policy (poll = report only, auto = redeploy on new digest) and optional check interval (Go duration, e.g. 24h). Stateful stacks (named volumes) default to poll.",
		ReadOnly:    false,
		Properties: map[string]any{
			"stack":    map[string]any{"type": "string", "description": "Stack name"},
			"policy":   map[string]any{"type": "string", "enum": []string{"poll", "auto"}, "description": "poll or auto"},
			"interval": map[string]any{"type": "string", "description": "Go duration string, e.g. 24h"},
		},
		Required: []string{"stack"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack := stringArg(args, "stack")
			if stack == "" {
				return "", "", nil, fmt.Errorf("set_update_policy requires argument \"stack\"")
			}
			body := map[string]any{}
			if p := stringArg(args, "policy"); p != "" {
				if p != "poll" && p != "auto" {
					return "", "", nil, fmt.Errorf("set_update_policy: policy must be poll or auto")
				}
				body["policy"] = p
			}
			if iv := stringArg(args, "interval"); iv != "" {
				body["interval"] = iv
			}
			if len(body) == 0 {
				return "", "", nil, fmt.Errorf("set_update_policy requires \"policy\" and/or \"interval\"")
			}
			return "PUT", "/update/" + url.PathEscape(stack) + "/policy", jsonBody(body), nil
		},
	},
	{
		Name:        "run_update",
		Description: "Force the snapshot-first auto-update flow now, regardless of policy: snapshot the stack with its volumes, redeploy health-gated, notify. Returns the run outcome including the snapshot path for rollback.",
		ReadOnly:    false,
		Destructive: true,
		Properties: map[string]any{
			"stack": map[string]any{"type": "string", "description": "Stack name"},
		},
		Required: []string{"stack"},
		build: func(args map[string]any) (string, string, []byte, error) {
			stack := stringArg(args, "stack")
			if stack == "" {
				return "", "", nil, fmt.Errorf("run_update requires argument \"stack\"")
			}
			return "POST", "/update/" + url.PathEscape(stack) + "/run", nil, nil
		},
	},
	{
		Name:        "get_audit",
		Description: "Read the operator audit log (newest first): who did what, when. Same view the UI's AuditLog card shows. Use it to verify an agent's own actions landed.",
		ReadOnly:    true,
		Properties: map[string]any{
			"limit":  map[string]any{"type": "integer", "description": "Entries to return (1..500, default 50)"},
			"actor":  map[string]any{"type": "string", "description": "Filter by actor (e.g. mcp, dev)"},
			"action": map[string]any{"type": "string", "description": "Filter by action (deploy, exec, update.run, ...)"},
			"since":  map[string]any{"type": "string", "description": "RFC 3339 timestamp; only entries at/after this"},
		},
		Required: nil,
		build: func(args map[string]any) (string, string, []byte, error) {
			limit, err := numberArg(args, "limit", 50)
			if err != nil {
				return "", "", nil, err
			}
			if limit < 1 || limit > 500 {
				return "", "", nil, fmt.Errorf("get_audit: limit must be 1..500")
			}
			frag := func(k, v string) string {
				if v == "" {
					return ""
				}
				return k + "=" + url.QueryEscape(v)
			}
			q := encodeQuery(intQuery("limit", limit), frag("actor", stringArg(args, "actor")), frag("action", stringArg(args, "action")), frag("since", stringArg(args, "since")))
			return "GET", "/audit" + q, nil, nil
		},
	},
	{
		Name:        "onboard_host",
		Description: "Zero-touch SSH onboarding: address + private key → Docker/Compose check+install → one Ferriage container, healthz-gated. The key is used in memory for one connection and never echoed back. Confirms nothing — never point this at a host that isn't yours.",
		ReadOnly:    false,
		Destructive: true,
		Properties: map[string]any{
			"address":     map[string]any{"type": "string", "description": "SSH address (host or host:port)"},
			"user":        map[string]any{"type": "string", "description": "SSH user (default root)"},
			"private_key": map[string]any{"type": "string", "description": "PEM-encoded SSH private key"},
			"image":       map[string]any{"type": "string", "description": "Peer image to start (optional)"},
			"peer_url":    map[string]any{"type": "string", "description": "URL the new peer pulls mesh state from (optional)"},
			"port":        map[string]any{"type": "integer", "description": "Peer port on the new host (optional)"},
		},
		Required: []string{"address", "private_key"},
		build: func(args map[string]any) (string, string, []byte, error) {
			address := stringArg(args, "address")
			key := stringArg(args, "private_key")
			if address == "" || key == "" {
				return "", "", nil, fmt.Errorf("onboard_host requires arguments \"address\" and \"private_key\"")
			}
			body := map[string]any{"address": address, "private_key": key}
			if u := stringArg(args, "user"); u != "" {
				body["user"] = u
			}
			if img := stringArg(args, "image"); img != "" {
				body["image"] = img
			}
			if pu := stringArg(args, "peer_url"); pu != "" {
				body["peer_url"] = pu
			}
			port, err := numberArg(args, "port", 0)
			if err != nil {
				return "", "", nil, err
			}
			if port > 0 {
				body["port"] = port
			}
			return "POST", "/hosts/onboard", jsonBody(body), nil
		},
	},
}

// findTool looks up a tool by name (nil when unknown).
func findTool(name string) *tool {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}

// toolList renders the tools[] array for tools/list: the
// inputSchema is a JSON Schema object per MCP, with
// readOnly/destructive hints so clients can gate calls.
func toolList() []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		props := t.Properties
		if props == nil {
			props = map[string]any{}
		}
		schema := map[string]any{"type": "object", "properties": props}
		if len(t.Required) > 0 {
			req := make([]any, len(t.Required))
			for i, r := range t.Required {
				req[i] = r
			}
			schema["required"] = req
		}
		ann := map[string]any{"readOnlyHint": t.ReadOnly, "destructiveHint": t.Destructive}
		out = append(out, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": schema,
			"annotations": ann,
		})
	}
	return out
}
