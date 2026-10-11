package proxy

import (
	"context"
	"netlite/internal/model"
)

// Domain configuration never stores Caddy's native configuration.
type ProxyEngine interface {
	Validate(context.Context, model.Config) error
	Apply(context.Context, model.Config) error
	Status(context.Context) (map[string]any, error)
}
