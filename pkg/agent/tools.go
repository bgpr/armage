package agent

import (
	"context"
)

// Tool represents a capability the agent can use.
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string) (string, error)
}

// Registry manages the available tools.
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}
