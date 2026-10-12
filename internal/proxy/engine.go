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
 Run(context.Context) error
 Applied()(model.Config,error)
 ReloadCertificates(context.Context)error
 Configuration(model.Config)(map[string]any,error)
}

