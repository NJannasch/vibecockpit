package builtin

import (
	"vibecockpit/internal/plugin"
	"vibecockpit/internal/provider"
)

type LocalPlugin struct {
	meta    plugin.Metadata
	prov    provider.Provider
	check   func() bool
	enabled bool
}

func New(id, name, icon string, prov provider.Provider, check func() bool) *LocalPlugin {
	return &LocalPlugin{
		meta: plugin.Metadata{
			ID:      id,
			Name:    name,
			Icon:    icon,
			Type:    "local",
			Version: "1.0.0",
			License: "free",
		},
		prov:  prov,
		check: check,
	}
}

func (p *LocalPlugin) Metadata() plugin.Metadata { return p.meta }
func (p *LocalPlugin) Provider() provider.Provider { return p.prov }
func (p *LocalPlugin) Enabled() bool               { return p.enabled }
func (p *LocalPlugin) SetEnabled(e bool)            { p.enabled = e }

func (p *LocalPlugin) Init(_ map[string]any) error {
	if p.check != nil && !p.check() {
		p.enabled = false
		return nil
	}
	p.enabled = true
	return nil
}
