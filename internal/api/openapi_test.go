package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// jsonFields returns the JSON field names of a struct type, in order.
func jsonFields(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		out = append(out, name)
	}
	return out
}

type schema struct {
	Required   []string        `json:"required"`
	Properties map[string]any  `json:"properties"`
	Items      json.RawMessage `json:"items"`
	Ref        string          `json:"$ref"`
}

func docSchemas(t *testing.T) map[string]schema {
	t.Helper()
	var doc struct {
		Components struct {
			Schemas map[string]schema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(replaceVersion(Document)), &doc); err != nil {
		t.Fatalf("openapi document is not valid JSON: %v", err)
	}
	return doc.Components.Schemas
}

// TestSchemasMatchGoStructs fails when the OpenAPI document names a
// field the Go struct does not (or vice versa). This is the drift
// guard that lets the web types be generated from the document.
func TestSchemasMatchGoStructs(t *testing.T) {
	schemas := docSchemas(t)
	cases := []struct {
		name string
		typ  reflect.Type
	}{
		{"Error", reflect.TypeOf(ErrorResponse{})},
		{"Peer", reflect.TypeOf(PeerResponse{})},
		{"Service", reflect.TypeOf(Service{})},
		{"Stack", reflect.TypeOf(Stack{})},
		{"Stacks", reflect.TypeOf(StacksResponse{})},
		{"DeploySvc", reflect.TypeOf(DeploySvc{})},
		{"DeployResult", reflect.TypeOf(DeployResponse{})},
		{"VolumeDesc", reflect.TypeOf(VolumeDesc{})},
		{"SnapshotManifest", reflect.TypeOf(SnapshotManifest{})},
		{"LogLine", reflect.TypeOf(LogLine{})},
		{"Logs", reflect.TypeOf(LogsResponse{})},
		{"ContainerStats", reflect.TypeOf(ContainerStats{})},
		{"Stats", reflect.TypeOf(StatsResponse{})},
		{"ExecRequest", reflect.TypeOf(ExecRequest{})},
		{"ExecResult", reflect.TypeOf(ExecResponse{})},
		{"ComposeFile", reflect.TypeOf(ComposeResponse{})},
		{"ComposeSave", reflect.TypeOf(ComposeSaveRequest{})},
		{"OnboardRequest", reflect.TypeOf(OnboardRequest{})},
		{"OnboardStep", reflect.TypeOf(OnboardStep{})},
		{"OnboardResponse", reflect.TypeOf(OnboardResponse{})},
		{"NotifyStatus", reflect.TypeOf(NotifyStatus{})},
		{"NotifyTest", reflect.TypeOf(NotifyTestResponse{})},
		{"AuditEntry", reflect.TypeOf(AuditEntry{})},
		{"Audit", reflect.TypeOf(AuditResponse{})},
		{"McpRequest", reflect.TypeOf(MCPRequest{})},
		{"McpResponse", reflect.TypeOf(MCPResponse{})},
		{"McpDescription", reflect.TypeOf(McpDescription{})},
	}
	for _, tc := range cases {
		s, ok := schemas[tc.name]
		if !ok {
			t.Errorf("schema %s missing from OpenAPI document", tc.name)
			continue
		}
		want := jsonFields(tc.typ)
		var got []string
		for k := range s.Properties {
			got = append(got, k)
		}
		// required is the document's view of omitempty-less fields;
		// compare as sets.
		reqSet := map[string]bool{}
		for _, r := range s.Required {
			reqSet[r] = true
		}
		for _, w := range want {
			if _, ok := s.Properties[w]; !ok {
				t.Errorf("%s: field %q in Go struct but not in schema", tc.name, w)
			}
		}
		for _, g := range got {
			found := false
			for _, w := range want {
				if w == g {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: field %q in schema but not in Go struct", tc.name, g)
			}
		}
		_ = reqSet
	}

	// Sync manifest wire (ADR 0001) — mirrored, not imported, to
	// avoid an import cycle with internal/sync.
	type syncEntry struct {
		Path      string    `json:"path"`
		SHA256    string    `json:"sha256"`
		Size      int64     `json:"size"`
		MTime     time.Time `json:"mtime"`
		UpdatedAt time.Time `json:"updated_at"`
		PeerID    string    `json:"peer_id"`
	}
	type syncManifest struct {
		PeerID      string      `json:"peer_id"`
		GeneratedAt time.Time   `json:"generated_at"`
		Entries     []syncEntry `json:"entries"`
	}
	for name, typ := range map[string]reflect.Type{
		"SyncEntry":    reflect.TypeOf(syncEntry{}),
		"SyncManifest": reflect.TypeOf(syncManifest{}),
	} {
		s, ok := schemas[name]
		if !ok {
			t.Errorf("schema %s missing from OpenAPI document", name)
			continue
		}
		for _, w := range jsonFields(typ) {
			if _, ok := s.Properties[w]; !ok {
				t.Errorf("%s: field %q in mirrored sync struct but not in schema", name, w)
			}
		}
		for k := range s.Properties {
			found := false
			for _, w := range jsonFields(typ) {
				if w == k {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: field %q in schema but not in mirrored sync struct", name, k)
			}
		}
	}
}

func TestHandlerServesDocument(t *testing.T) {
	h := Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /openapi.json = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"openapi": "3.1.0"`) {
		t.Errorf("document is not OpenAPI 3.1")
	}
	if strings.Contains(body, "VERSION") {
		t.Errorf("version placeholder leaked into the served document")
	}
	if !strings.Contains(body, "/stacks/{name}/deploy") {
		t.Errorf("deploy path missing from the document")
	}
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /openapi.yaml = %d", rec2.Code)
	}
}
