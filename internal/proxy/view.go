package proxy

import (
	"encoding/json"
	"netlite/internal/model"
	"os"
	"path/filepath"
	"strings"
)

// Configuration exposes only the generated draft and last persisted checkpoint.
// It never expands credential environment references or queries another process.
func (c *Caddy) Configuration(cfg model.Config) (map[string]any, error) {
	draft, err := c.Render(cfg)
	if err != nil {
		return nil, err
	}
	var d, a any
	if err = json.Unmarshal(draft, &d); err != nil {
		return nil, err
	}
	applied, err := os.ReadFile(filepath.Join(c.Data, "applied.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(applied) > 0 {
		if err = json.Unmarshal(applied, &a); err != nil {
			return nil, err
		}
	}
	return map[string]any{"draft": redact(d), "applied": redact(a)}, nil
}

func redact(v any) any {
	switch value := v.(type) {
	case map[string]any:
		for key, item := range value {
			name := strings.ToLower(key)
			sensitive := strings.Contains(name, "token") || strings.Contains(name, "password") || strings.Contains(name, "secret") || strings.Contains(name, "private_key") || name == "api_key"
			if sensitive {
				if s, ok := item.(string); ok && strings.HasPrefix(s, "{env.") && strings.HasSuffix(s, "}") {
					continue
				}
				value[key] = "[REDACTED]"
			} else {
				value[key] = redact(item)
			}
		}
	case []any:
		for i, item := range value {
			value[i] = redact(item)
		}
	}
	return v
}
