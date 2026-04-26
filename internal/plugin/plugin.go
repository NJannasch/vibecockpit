package plugin

import (
	"context"
	"net/http"

	"vibecockpit/internal/provider"
)

type Metadata struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Icon        string `json:"icon" yaml:"icon"`
	Type        string `json:"type" yaml:"type"` // "local", "remote", "api"
	Version     string `json:"version" yaml:"version"`
	License     string `json:"license" yaml:"license"` // "free", "commercial"
}

type Route struct {
	Path    string
	Handler http.HandlerFunc
}

type Plugin interface {
	Metadata() Metadata
	Init(cfg map[string]any) error
	Provider() provider.Provider
	Enabled() bool
	SetEnabled(bool)
}

type UIPlugin interface {
	Plugin
	Routes() []Route
}

type LifecyclePlugin interface {
	Plugin
	Start(ctx context.Context) error
	Stop() error
}
