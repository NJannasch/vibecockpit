package plugin

import (
	"context"
	"fmt"
	"sync"

	"vibecockpit/internal/config"
	"vibecockpit/internal/provider"
)

type Registry struct {
	mu      sync.RWMutex
	plugins []Plugin
	cfg     *config.Config
}

func NewRegistry(cfg *config.Config) *Registry {
	return &Registry{cfg: cfg}
}

func (r *Registry) Register(p Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = append(r.plugins, p)
}

func (r *Registry) InitAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	disabledSet := make(map[string]bool)
	for _, name := range r.cfg.DisabledProviders {
		disabledSet[name] = true
	}

	pluginConfigs := r.cfg.PluginConfigs
	if pluginConfigs == nil {
		pluginConfigs = make(map[string]map[string]any)
	}

	for _, p := range r.plugins {
		meta := p.Metadata()

		if disabledSet[meta.ID] {
			p.SetEnabled(false)
			continue
		}

		cfg := pluginConfigs[meta.ID]
		if cfg == nil {
			cfg = make(map[string]any)
		}
		if err := p.Init(cfg); err != nil {
			p.SetEnabled(false)
			continue
		}
	}

	return nil
}

func (r *Registry) StartAll(ctx context.Context) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if !p.Enabled() {
			continue
		}
		if lp, ok := p.(LifecyclePlugin); ok {
			go func() { _ = lp.Start(ctx) }()
		}
	}
}

func (r *Registry) StopAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if lp, ok := p.(LifecyclePlugin); ok {
			_ = lp.Stop()
		}
	}
}

func (r *Registry) Providers() []provider.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var providers []provider.Provider
	for _, p := range r.plugins {
		if p.Enabled() {
			providers = append(providers, p.Provider())
		}
	}
	return providers
}

func (r *Registry) Plugins() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plugin, len(r.plugins))
	copy(out, r.plugins)
	return out
}

func (r *Registry) Routes() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var routes []Route
	for _, p := range r.plugins {
		if !p.Enabled() {
			continue
		}
		if up, ok := p.(UIPlugin); ok {
			routes = append(routes, up.Routes()...)
		}
	}
	return routes
}

func (r *Registry) Get(id string) (Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if p.Metadata().ID == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("plugin %q not found", id)
}

func (r *Registry) SetEnabled(id string, enabled bool) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	p.SetEnabled(enabled)
	return nil
}
