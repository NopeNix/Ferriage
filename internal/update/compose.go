package update

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// renderCompose renders a stack's Compose file to canonical JSON
// (`docker compose config --format json`). The stateful heuristic
// runs on the rendered model, not on YAML text, so comments and
// anchor tricks cannot fake a volume.
func renderCompose(ctx context.Context, stackDir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", "compose.yaml", "-p", filepath.Base(stackDir), "config", "--format", "json")
	cmd.Dir = stackDir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// renderedHasNamedVolume reports whether the rendered Compose
// model has any named volume: either a top-level volumes entry,
// or a service volume with type "volume" (a named volume
// referenced only from a service is still state).
func renderedHasNamedVolume(data []byte) bool {
	var doc struct {
		Volumes  map[string]any `json:"volumes"`
		Services map[string]struct {
			Volumes []struct {
				Type string `json:"type"`
			} `json:"volumes"`
		} `json:"services"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	if len(doc.Volumes) > 0 {
		return true
	}
	for _, s := range doc.Services {
		for _, v := range s.Volumes {
			if v.Type == "volume" {
				return true
			}
		}
	}
	return false
}

// textHasNamedVolume is the fallback heuristic when rendering
// fails (no Docker, bad file): a top-level `volumes:` key with at
// least one named entry. Deliberately conservative — a render
// failure must not make a stateful stack look stateless and
// inherit `auto`.
func textHasNamedVolume(composePath string) bool {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return false
	}
	inVolumes := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "volumes:") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inVolumes = true
			continue
		}
		if inVolumes {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				if strings.HasSuffix(trimmed, ":") || strings.Contains(trimmed, ":") {
					// A key under volumes: → named volume.
					return true
				}
				continue
			}
			if trimmed == "" {
				continue
			}
			inVolumes = false
		}
	}
	return false
}
