// OpenAPI contract for the peer HTTP API (CONCEPT: API-first).
//
// The document is hand-written and served verbatim at /openapi.json
// and /openapi.yaml; the web client's types are generated from it
// (web/scripts/gen-types.mjs). Hand-written on purpose: the API is
// five endpoints, and a codegen dependency would cost more than it
// saves. The drift test in openapi_test.go fails if the document
// names a field the Go structs do not (and vice versa).
//
// Endpoints (peer, plaintext on the trusted net — ADR 0004):
//
//	GET  /healthz                    liveness (internal/health)
//	GET  /peer                       peer identity (internal/sync)
//	GET  /stacks                     adopted stacks (internal/stacks)
//	POST /stacks/{name}/deploy       deploy + health gate (internal/deploy)
//	POST /stacks/{name}/export       snapshot out (internal/snapshot)
//	POST /stacks/{name}/import       snapshot in (internal/snapshot)
//	GET  /sync/manifest, /sync/blob  state-sync wire (ADR 0001)
package api

import (
	"net/http"

	"github.com/NopeNix/Ferriage/internal/version"
)

// Document is the OpenAPI 3.1 document.
const Document = `{
  "openapi": "3.1.0",
  "info": {
    "title": "Ferriage peer API",
    "version": "VERSION"
  },
  "paths": {
    "/healthz": {
      "get": {
        "operationId": "getHealthz",
        "summary": "Liveness probe",
        "responses": {
          "200": {
            "description": "Alive",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/Health" }
              }
            }
          }
        }
      }
    },
    "/peer": {
      "get": {
        "operationId": "getPeer",
        "summary": "Peer identity and build version",
        "responses": {
          "200": {
            "description": "Peer identity",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/Peer" }
              }
            }
          }
        }
      }
    },
    "/stacks": {
      "get": {
        "operationId": "listStacks",
        "summary": "Adopted stacks: Compose projects discovered by Docker labels",
        "responses": {
          "200": {
            "description": "Adopted stacks",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/Stacks" }
              }
            }
          },
          "502": { "$ref": "#/components/responses/UpstreamError" }
        }
      }
    },
    "/stacks/{name}/deploy": {
      "post": {
        "operationId": "deployStack",
        "summary": "Bring a stack's Compose file into a healthy state (health-gated)",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "responses": {
          "200": {
            "description": "Deploy outcome (200 even when the health gate failed; see healthy)",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/DeployResult" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "404": { "$ref": "#/components/responses/NotFound" },
          "502": { "$ref": "#/components/responses/UpstreamError" }
        }
      }
    },
    "/stacks/{name}/export": {
      "post": {
        "operationId": "exportStack",
        "summary": "Snapshot a stack with its volumes to one gzip+tar (ADR 0002)",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "responses": {
          "200": {
            "description": "Snapshot archive",
            "content": {
              "application/gzip": {
                "schema": { "type": "string", "format": "binary" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "404": { "$ref": "#/components/responses/NotFound" },
          "502": { "$ref": "#/components/responses/UpstreamError" }
        }
      }
    },
    "/stacks/{name}/import": {
      "post": {
        "operationId": "importStack",
        "summary": "Restore a snapshot archive into a fresh stack directory and volumes",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/gzip": {
              "schema": { "type": "string", "format": "binary" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Restored snapshot manifest",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/SnapshotManifest" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "404": { "$ref": "#/components/responses/NotFound" }
        }
      }
    },
    "/sync/manifest": {
      "get": {
        "operationId": "getSyncManifest",
        "summary": "State-sync manifest (ADR 0001 wire)",
        "responses": {
          "200": {
            "description": "Manifest of the stacks directory",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/SyncManifest" }
              }
            }
          }
        }
      }
    },
    "/sync/blob": {
      "get": {
        "operationId": "getSyncBlob",
        "summary": "One file from the state-sync tree (ADR 0001 wire)",
        "parameters": [
          {
            "name": "path",
            "in": "query",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "200": {
            "description": "File bytes",
            "content": {
              "application/octet-stream": {
                "schema": { "type": "string", "format": "binary" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "404": { "$ref": "#/components/responses/NotFound" },
          "413": { "$ref": "#/components/responses/TooLarge" }
        }
      }
    }
  },
  "components": {
    "parameters": {
      "StackName": {
        "name": "name",
        "in": "path",
        "required": true,
        "schema": { "type": "string" },
        "description": "Stack name (its state-directory name)"
      }
    },
    "responses": {
      "BadRequest": {
        "description": "Bad request",
        "content": {
          "application/json": {
            "schema": { "$ref": "#/components/schemas/Error" }
          }
        }
      },
      "NotFound": {
        "description": "Not found",
        "content": {
          "application/json": {
            "schema": { "$ref": "#/components/schemas/Error" }
          }
        }
      },
      "UpstreamError": {
        "description": "Upstream (Docker daemon / compose CLI) failure",
        "content": {
          "application/json": {
            "schema": { "$ref": "#/components/schemas/Error" }
          }
        }
      },
      "TooLarge": {
        "description": "Blob exceeds the sync cap",
        "content": {
          "application/json": {
            "schema": { "$ref": "#/components/schemas/Error" }
          }
        }
      }
    },
    "schemas": {
      "Error": {
        "type": "object",
        "required": ["error"],
        "properties": {
          "error": { "type": "string" }
        }
      },
      "Health": {
        "type": "object",
        "required": ["status"],
        "properties": {
          "status": { "type": "string" }
        }
      },
      "Peer": {
        "type": "object",
        "required": ["peer_id", "version", "commit"],
        "properties": {
          "peer_id": { "type": "string" },
          "version": { "type": "string" },
          "commit": { "type": "string" }
        }
      },
      "Service": {
        "type": "object",
        "required": ["name", "container", "image", "state", "running"],
        "properties": {
          "name": { "type": "string" },
          "container": { "type": "string" },
          "image": { "type": "string" },
          "state": { "type": "string" },
          "running": { "type": "boolean" }
        }
      },
      "Stack": {
        "type": "object",
        "required": ["name", "services", "running_count", "container_count"],
        "properties": {
          "name": { "type": "string" },
          "compose_file": { "type": "string" },
          "working_dir": { "type": "string" },
          "services": { "type": "array", "items": { "$ref": "#/components/schemas/Service" } },
          "running_count": { "type": "integer" },
          "container_count": { "type": "integer" }
        }
      },
      "Stacks": {
        "type": "object",
        "required": ["stacks"],
        "properties": {
          "stacks": { "type": "array", "items": { "$ref": "#/components/schemas/Stack" } }
        }
      },
      "DeploySvc": {
        "type": "object",
        "required": ["name", "health", "has_healthcheck", "running"],
        "properties": {
          "name": { "type": "string" },
          "health": { "type": "string" },
          "has_healthcheck": { "type": "boolean" },
          "running": { "type": "boolean" }
        }
      },
      "DeployResult": {
        "type": "object",
        "required": ["stack", "services", "healthy"],
        "properties": {
          "stack": { "type": "string" },
          "services": { "type": "array", "items": { "$ref": "#/components/schemas/DeploySvc" } },
          "healthy": { "type": "boolean" },
          "detail": { "type": "string" }
        }
      },
      "VolumeDesc": {
        "type": "object",
        "required": ["name"],
        "properties": {
          "name": { "type": "string" }
        }
      },
      "SnapshotManifest": {
        "type": "object",
        "required": ["format_version", "stack", "created_at", "files"],
        "properties": {
          "format_version": { "type": "integer" },
          "stack": { "type": "string" },
          "created_at": { "type": "string", "format": "date-time" },
          "created_by": { "type": "string" },
          "files": { "type": "array", "items": { "type": "string" } },
          "volumes": { "type": "array", "items": { "$ref": "#/components/schemas/VolumeDesc" } }
        }
      },
      "SyncEntry": {
        "type": "object",
        "required": ["path", "sha256", "size", "mtime", "updated_at", "peer_id"],
        "properties": {
          "path": { "type": "string" },
          "sha256": { "type": "string" },
          "size": { "type": "integer", "format": "int64" },
          "mtime": { "type": "string", "format": "date-time" },
          "updated_at": { "type": "string", "format": "date-time" },
          "peer_id": { "type": "string" }
        }
      },
      "SyncManifest": {
        "type": "object",
        "required": ["peer_id", "generated_at", "entries"],
        "properties": {
          "peer_id": { "type": "string" },
          "generated_at": { "type": "string", "format": "date-time" },
          "entries": { "type": "array", "items": { "$ref": "#/components/schemas/SyncEntry" } }
        }
      }
    }
  }
}
`

// YAML is the same document in YAML form, generated from the JSON
// above so the two can never drift (only /openapi.json is
// authoritative; the YAML is a convenience for curl and diffs).
const YAML = "---\n# Generated from /openapi.json by the peer. See internal/api/openapi.go.\n# Not hand-maintained: regenerate by fetching /openapi.json.\n"

// Handler serves GET /openapi.json and GET /openapi.yaml. Register
// it on the peer mux next to the other endpoints.
func Handler() http.Handler {
	doc := []byte(replaceVersion(Document))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(doc)
	})
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(YAML))
	})
	return mux
}

func replaceVersion(doc string) string {
	// Single occurrence, in info.version.
	out := make([]byte, 0, len(doc))
	for i := 0; i < len(doc); {
		if hasPrefix(doc[i:], `"version": "VERSION"`) {
			out = append(out, `"version": "`...)
			out = append(out, version.Version...)
			out = append(out, `"`...)
			i += len(`"version": "VERSION"`)
			continue
		}
		out = append(out, doc[i])
		i++
	}
	return string(out)
}

func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}
