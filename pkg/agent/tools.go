package agent

import (
	"context"
	"fmt"
)

// Tool represents a capability the agent can use.
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string) (string, error)
	Preview(ctx context.Context, args string) (string, error) // Returns a diff or preview of the action
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

// Truncate ensures tool output doesn't exceed a reasonable limit for LLM context and TUI rendering.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n... (truncated %d bytes)", len(s)-max)
}
