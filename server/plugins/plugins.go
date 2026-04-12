package plugins

import (
	"github.com/webdeveloperben/tyche/server"
)

type Plugin interface {
	Register(r *server.Router) error
}

type PluginRegistry struct {
	plugins []Plugin
}

func NewRegistry() *PluginRegistry {
	return &PluginRegistry{}
}

func Register(router *server.Router, plugins ...Plugin) error {
	return NewRegistry().AddAll(plugins...).Register(router)
}

func (r *PluginRegistry) Add(p Plugin) *PluginRegistry {
	r.plugins = append(r.plugins, p)
	return r
}

func (r *PluginRegistry) AddAll(plugins ...Plugin) *PluginRegistry {
	r.plugins = append(r.plugins, plugins...)
	return r
}

func (r *PluginRegistry) Register(router *server.Router) error {
	for _, p := range r.plugins {
		if err := p.Register(router); err != nil {
			return err
		}
	}
	return nil
}
