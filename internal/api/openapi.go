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
//	GET  /stacks/{name}/services/{svc}/logs  tails (internal/logs)
//	GET  /stacks/{name}/stats        live stats (internal/stats)
//	POST /stacks/{name}/services/{svc}/exec  one-shot exec (internal/exec)
//	GET|PUT /stacks/{name}/compose   compose file CRUD (internal/compose)
//	WS   /stacks/{name}/services/{svc}/console  live exec console (internal/console)
//	POST /stacks/{name}/export       snapshot out (internal/snapshot)
//	POST /stacks/{name}/import       snapshot in (internal/snapshot)
//	POST /hosts/onboard              SSH onboarding (internal/onboard, ADR 0006)
//	GET  /notify, POST /notify/test  redacted Shoutrrr status/test (internal/notify, ADR 0007)
//	GET  /audit                      audit log, newest first (internal/audit, ADR 0008)
//	GET  /update                     auto-update reports (internal/update, ADR 0009)
//	GET  /update/{name}              one stack's update report
//	POST /update/{name}/check        force a check now
//	PUT  /update/{name}/policy       set a stack's policy
//	POST /update/{name}/run          snapshot + deploy now
//	POST /mcp                        MCP Streamable HTTP (ADR 0010)
//	GET/DELETE /mcp                  MCP surface description / stateless close
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
    "/stacks/{name}/services/{svc}/logs": {
      "get": {
        "operationId": "getServiceLogs",
        "summary": "Tail of one container's logs (non-follow, bounded)",
        "parameters": [
          { "$ref": "#/components/parameters/StackName" },
          { "$ref": "#/components/parameters/ServiceName" },
          { "name": "tail", "in": "query", "schema": { "type": "integer", "minimum": 1, "maximum": 10000, "default": 200 } },
          { "name": "since", "in": "query", "schema": { "type": "string" } }
        ],
        "responses": {
          "200": {
            "description": "Log lines",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/Logs" }
              }
            }
          },
          "404": { "$ref": "#/components/responses/NotFound" },
          "502": { "$ref": "#/components/responses/UpstreamError" }
        }
      }
    },
    "/stacks/{name}/stats": {
      "get": {
        "operationId": "getStackStats",
        "summary": "One-shot live stats snapshot for every container of a stack",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "responses": {
          "200": {
            "description": "Stats snapshot",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/Stats" }
              }
            }
          },
          "404": { "$ref": "#/components/responses/NotFound" },
          "502": { "$ref": "#/components/responses/UpstreamError" }
        }
      }
    },
    "/stacks/{name}/services/{svc}/exec": {
      "post": {
        "operationId": "execInService",
        "summary": "Run a one-shot command in one running container",
        "parameters": [
          { "$ref": "#/components/parameters/StackName" },
          { "$ref": "#/components/parameters/ServiceName" }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/ExecRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Exec outcome",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ExecResult" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "404": { "$ref": "#/components/responses/NotFound" },
          "502": { "$ref": "#/components/responses/UpstreamError" }
        }
      }
    },
    "/stacks/{name}/services/{svc}/console": {
      "get": {
        "operationId": "consoleService",
        "summary": "Interactive exec console over WebSocket (binary frames = terminal bytes; JSON text frames = control: resize/close)",
        "parameters": [
          { "$ref": "#/components/parameters/StackName" },
          { "$ref": "#/components/parameters/ServiceName" }
        ],
        "responses": {
          "101": { "description": "Socket upgrade; one exec session per connection" }
        }
      }
    },
    "/stacks/{name}/compose": {
      "get": {
        "operationId": "getStackCompose",
        "summary": "Read a stack's Compose file",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "responses": {
          "200": {
            "description": "Compose file content",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ComposeFile" }
              }
            }
          },
          "404": { "$ref": "#/components/responses/NotFound" }
        }
      },
      "put": {
        "operationId": "putStackCompose",
        "summary": "Write a stack's Compose file (full replace, atomic)",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/ComposeSave" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Saved Compose file",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ComposeFile" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "404": { "$ref": "#/components/responses/NotFound" },
          "413": { "$ref": "#/components/responses/TooLarge" }
        }
      }
    },
    "/hosts/onboard": {
      "post": {
        "operationId": "onboardHost",
        "summary": "Zero-touch SSH onboarding: address + key, Docker check, one Ferriage container (ADR 0006)",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/OnboardRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Onboarded and healthy",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/OnboardResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "502": { "$ref": "#/components/responses/UpstreamError" }
        }
      }
    },
    "/notify": {
      "get": {
        "operationId": "getNotifyStatus",
        "summary": "Redacted notification status (enabled, channel count, last send outcome — never the URLs, ADR 0007)",
        "responses": {
          "200": {
            "description": "Notification status",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/NotifyStatus" }
              }
            }
          }
        }
      }
    },
    "/notify/test": {
      "post": {
        "operationId": "sendNotifyTest",
        "summary": "Send one synchronous test notification (503 when unconfigured, 502 when delivery fails)",
        "responses": {
          "200": {
            "description": "Test notification delivered",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/NotifyTest" }
              }
            }
          },
          "502": { "$ref": "#/components/responses/UpstreamError" },
          "503": { "$ref": "#/components/responses/BadRequest" }
        }
      }
    },
    "/audit": {
      "get": {
        "operationId": "listAudit",
        "summary": "Audit log entries, newest first (ADR 0008: who did what, when). Read-only; there is no write or delete endpoint.",
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer", "minimum": 1, "maximum": 500, "default": 50 } },
          { "name": "actor", "in": "query", "schema": { "type": "string" } },
          { "name": "action", "in": "query", "schema": { "type": "string" } },
          { "name": "since", "in": "query", "schema": { "type": "string", "format": "date-time" } }
        ],
        "responses": {
          "200": {
            "description": "Audit entries (newest first)",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/Audit" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" }
        }
      }
    },
    "/update": {
      "get": {
        "operationId": "listUpdate",
        "summary": "Auto-update reports for every stack (ADR 0009: per-node digest poll, poll vs auto)",
        "responses": {
          "200": {
            "description": "Update reports",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/UpdateList" }
              }
            }
          }
        }
      }
    },
    "/update/{name}": {
      "get": {
        "operationId": "getUpdate",
        "summary": "One stack's update report",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "responses": {
          "200": {
            "description": "Update report",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/UpdateStatus" }
              }
            }
          },
          "404": { "$ref": "#/components/responses/NotFound" }
        }
      }
    },
    "/update/{name}/check": {
      "post": {
        "operationId": "checkUpdate",
        "summary": "Force a synchronous digest check for one stack (never deploys; not audited)",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "responses": {
          "200": {
            "description": "Fresh update report",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/UpdateStatus" }
              }
            }
          },
          "404": { "$ref": "#/components/responses/NotFound" },
          "502": { "$ref": "#/components/responses/UpstreamError" }
        }
      }
    },
    "/update/{name}/policy": {
      "put": {
        "operationId": "setUpdatePolicy",
        "summary": "Set a stack's update policy (poll or auto) and check interval",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/UpdatePolicyRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Effective policy after the write",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/UpdatePolicyResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "404": { "$ref": "#/components/responses/NotFound" }
        }
      }
    },
    "/update/{name}/run": {
      "post": {
        "operationId": "runUpdate",
        "summary": "Force the snapshot-first auto-update flow now, regardless of policy",
        "parameters": [{ "$ref": "#/components/parameters/StackName" }],
        "responses": {
          "200": {
            "description": "Run outcome",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/UpdateRun" }
              }
            }
          },
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
    },
    "/mcp": {
      "post": {
        "operationId": "mcpCall",
        "summary": "MCP Streamable HTTP endpoint (ADR 0010): JSON-RPC 2.0 for tools/list, tools/call, initialize, ping",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/McpRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "JSON-RPC response (or array of responses for a batch)",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/McpResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" }
        }
      },
      "get": {
        "operationId": "mcpDescribe",
        "summary": "MCP surface description (tool inventory for non-MCP clients)",
        "responses": {
          "200": {
            "description": "Surface description",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/McpDescription" }
              }
            }
          }
        }
      },
      "delete": {
        "operationId": "mcpClose",
        "summary": "Stateless close (no session to tear down; 202)",
        "responses": {
          "202": { "description": "Accepted" }
        }
      }
    }
  },
  "components": {
    "parameters": {
      "ServiceName": {
        "name": "svc",
        "in": "path",
        "required": true,
        "schema": { "type": "string" },
        "description": "Compose service name within the stack"
      },
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
      "LogLine": {
        "type": "object",
        "required": ["stream", "message"],
        "properties": {
          "ts": { "type": "string" },
          "stream": { "type": "string" },
          "message": { "type": "string" }
        }
      },
      "Logs": {
        "type": "object",
        "required": ["container", "lines", "truncated"],
        "properties": {
          "container": { "type": "string" },
          "lines": { "type": "array", "items": { "$ref": "#/components/schemas/LogLine" } },
          "truncated": { "type": "boolean" }
        }
      },
      "ContainerStats": {
        "type": "object",
        "required": ["container", "cpu_percent", "memory_bytes", "memory_limit", "memory_percent", "net_rx_bytes", "net_tx_bytes"],
        "properties": {
          "container": { "type": "string" },
          "cpu_percent": { "type": "number" },
          "memory_bytes": { "type": "integer", "format": "int64" },
          "memory_limit": { "type": "integer", "format": "int64" },
          "memory_percent": { "type": "number" },
          "net_rx_bytes": { "type": "integer", "format": "int64" },
          "net_tx_bytes": { "type": "integer", "format": "int64" }
        }
      },
      "Stats": {
        "type": "object",
        "required": ["stats"],
        "properties": {
          "stats": { "type": "array", "items": { "$ref": "#/components/schemas/ContainerStats" } }
        }
      },
      "ExecRequest": {
        "type": "object",
        "required": ["cmd"],
        "properties": {
          "cmd": { "type": "array", "items": { "type": "string" } },
          "tty": { "type": "boolean" }
        }
      },
      "ExecResult": {
        "type": "object",
        "required": ["exec_id", "running", "exit_code", "output", "truncated"],
        "properties": {
          "exec_id": { "type": "string" },
          "running": { "type": "boolean" },
          "exit_code": { "type": "integer" },
          "output": { "type": "string" },
          "truncated": { "type": "boolean" }
        }
      },
      "OnboardRequest": {
        "type": "object",
        "required": ["address", "private_key"],
        "properties": {
          "address": { "type": "string" },
          "user": { "type": "string" },
          "private_key": { "type": "string" },
          "image": { "type": "string" },
          "peer_url": { "type": "string" },
          "port": { "type": "integer" }
        }
      },
      "OnboardStep": {
        "type": "object",
        "required": ["name", "ok"],
        "properties": {
          "name": { "type": "string" },
          "ok": { "type": "boolean" },
          "detail": { "type": "string" }
        }
      },
      "OnboardResponse": {
        "type": "object",
        "required": ["address", "docker", "compose", "container", "peer_url", "healthy", "steps"],
        "properties": {
          "address": { "type": "string" },
          "docker": { "type": "string" },
          "compose": { "type": "string" },
          "container": { "type": "string" },
          "peer_url": { "type": "string" },
          "healthy": { "type": "boolean" },
          "steps": { "type": "array", "items": { "$ref": "#/components/schemas/OnboardStep" } },
          "error": { "type": "string" }
        }
      },
      "NotifyStatus": {
        "type": "object",
        "required": ["enabled", "channels"],
        "properties": {
          "enabled": { "type": "boolean" },
          "channels": { "type": "integer" },
          "last_event": { "type": "string" },
          "last_at": { "type": "string" },
          "last_error": { "type": "string" }
        }
      },
      "NotifyTest": {
        "type": "object",
        "required": ["sent", "event"],
        "properties": {
          "sent": { "type": "boolean" },
          "event": { "type": "string" }
        }
      },
      "AuditEntry": {
        "type": "object",
        "required": ["ts", "actor", "action", "status"],
        "properties": {
          "ts": { "type": "string", "format": "date-time" },
          "actor": { "type": "string" },
          "action": { "type": "string" },
          "target": { "type": "string" },
          "status": { "type": "integer" },
          "detail": { "type": "string" }
        }
      },
      "Audit": {
        "type": "object",
        "required": ["entries"],
        "properties": {
          "entries": { "type": "array", "items": { "$ref": "#/components/schemas/AuditEntry" } }
        }
      },
      "UpdateService": {
        "type": "object",
        "required": ["service", "image", "outcome"],
        "properties": {
          "service": { "type": "string" },
          "image": { "type": "string" },
          "local": { "type": "string" },
          "remote": { "type": "string" },
          "outcome": { "type": "string" },
          "detail": { "type": "string" }
        }
      },
      "UpdateRun": {
        "type": "object",
        "required": ["stack", "at", "ok"],
        "properties": {
          "stack": { "type": "string" },
          "at": { "type": "string", "format": "date-time" },
          "snapshot": { "type": "string" },
          "ok": { "type": "boolean" },
          "detail": { "type": "string" }
        }
      },
      "UpdateStatus": {
        "type": "object",
        "required": ["stack", "state", "policy", "stateful", "services"],
        "properties": {
          "stack": { "type": "string" },
          "state": { "type": "string" },
          "policy": { "type": "string" },
          "stateful": { "type": "boolean" },
          "checked_at": { "type": "string", "format": "date-time" },
          "services": { "type": "array", "items": { "$ref": "#/components/schemas/UpdateService" } },
          "last_run": { "$ref": "#/components/schemas/UpdateRun" },
          "snapshot": { "type": "string" },
          "error": { "type": "string" }
        }
      },
      "UpdateList": {
        "type": "object",
        "required": ["stacks"],
        "properties": {
          "stacks": { "type": "array", "items": { "$ref": "#/components/schemas/UpdateStatus" } }
        }
      },
      "UpdatePolicyRequest": {
        "type": "object",
        "properties": {
          "policy": { "type": "string", "enum": ["poll", "auto"] },
          "interval": { "type": "string" }
        }
      },
      "UpdatePolicyResponse": {
        "type": "object",
        "required": ["stack", "policy", "interval", "stateful"],
        "properties": {
          "stack": { "type": "string" },
          "policy": { "type": "string" },
          "interval": { "type": "string" },
          "stateful": { "type": "boolean" }
        }
      },
      "ComposeFile": {
        "type": "object",
        "required": ["name", "file", "content"],
        "properties": {
          "name": { "type": "string" },
          "file": { "type": "string" },
          "content": { "type": "string" },
          "compose_file_used": { "type": "string" }
        }
      },
      "ComposeSave": {
        "type": "object",
        "required": ["content"],
        "properties": {
          "content": { "type": "string" }
        }
      },
      "McpRequest": {
        "type": "object",
        "required": ["jsonrpc", "id", "method"],
        "properties": {
          "jsonrpc": { "type": "string" },
          "id": { "type": ["string", "integer", "null"] },
          "method": { "type": "string" },
          "params": { "type": "object" }
        }
      },
      "McpResponse": {
        "type": "object",
        "required": ["jsonrpc", "id"],
        "properties": {
          "jsonrpc": { "type": "string" },
          "id": { "type": ["string", "integer", "null"] },
          "result": { "type": "object" },
          "error": { "type": "object" }
        }
      },
      "McpDescription": {
        "type": "object",
        "required": ["server", "protocol", "transport", "stateless", "tools", "doc"],
        "properties": {
          "server": { "type": "object" },
          "protocol": { "type": "string" },
          "transport": { "type": "string" },
          "stateless": { "type": "boolean" },
          "tools": { "type": "array", "items": { "type": "string" } },
          "doc": { "type": "string" }
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
