package proxy

import (
	"encoding/json"
	"netlite/internal/model"
	"strings"
)

// Configuration exposes only the generated draft and last persisted checkpoint.
// It never expands credential environment references or queries another process.
func (c *Caddy) Configuration(cfg model.Config) (map[string]any, error) {
	out := map[string]any{"draft": nil, "applied": nil}
	draft, err := c.Render(cfg)
	if err != nil {
		out["draft_error"] = err.Error()
	} else {
		var d any
		if err = json.Unmarshal(draft, &d); err != nil {
			return nil, err
		}
		out["draft"] = d
	}
	applied, err := c.Applied()
	if err != nil {
		return nil, err
	}
	if len(applied.Routes) > 0 {
		b, err := c.Render(applied)
		if err != nil {
			return nil, err
		}
		var a any
		if err = json.Unmarshal(b, &a); err != nil {
			return nil, err
		}
		out["applied"] = a
	}
	return out, nil
}

func redact(v any) any {
	switch value := v.(type) {
	case map[string]any:
		for key, item := range value {
			name := strings.ToLower(key)
			sensitive := strings.Contains(name, "token") || strings.Contains(name, "password") || strings.Contains(name, "secret") || strings.Contains(name, "private_key") || name == "api_key"
			if sensitive {
				if s, ok := item.(string); ok && (strings.HasPrefix(s, "{env.") || strings.HasPrefix(s, "{netlite.dns.")) && strings.HasSuffix(s, "}") {
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
