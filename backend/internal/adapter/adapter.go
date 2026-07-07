// Package adapter defines the unified interface that all CLI agent adapters
// implement. Each adapter spawns a single subprocess per session and streams
// stdout/stderr line-by-line back to the caller.
package adapter

import (
	"context"
	"io"
)

// Event is a single stream chunk produced by an adapter.
type Event struct {
	// Type is one of: "text" | "tool" | "error" | "done"
	Type string `json:"type"`
	// Text holds the textual content for "text" and "error" events.
	Text string `json:"text,omitempty"`
	// ToolName is set for "tool" events (e.g. "Read", "Edit", "Bash").
	ToolName string `json:"tool_name,omitempty"`
	// ToolInput is a short string preview of tool input, optional.
	ToolInput string `json:"tool_input,omitempty"`
}

// Spec is the per-invocation configuration.
type Spec struct {
	// Prompt is the user message text (may be multi-line).
	Prompt string
	// ProjectDir is the working directory (absolute path, already validated).
	ProjectDir string
	// Model overrides the adapter default; empty = adapter default.
	Model string
	// ExtraArgs lets the API layer pass adapter-specific flags.
	ExtraArgs []string
	// Env is additional env (KEY=VAL pairs) merged onto os.Environ.
	Env []string
}

// Adapter is the interface implemented by each CLI agent.
type Adapter interface {
	// ID returns the adapter identifier ("claude", "codex", ...).
	ID() string
	// DisplayName is the human label shown in UI.
	DisplayName() string
	// Available reports whether the underlying CLI binary is on PATH.
	Available(ctx context.Context) bool
	// Run executes the agent and streams events to the writer.
	// The implementation MUST respect ctx cancellation and close the writer
	// by sending a final Event{Type:"done"}.
	Run(ctx context.Context, spec Spec, w EventWriter) error
}

// EventWriter is implemented by the chat session. Implementations must be
// safe for concurrent use from the streaming goroutine.
type EventWriter interface {
	WriteEvent(Event) error
	io.Closer
}

// Registry holds all known adapters keyed by ID.
type Registry struct {
	adapters map[string]Adapter
	order    []string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{adapters: make(map[string]Adapter)} }

// Register adds an adapter.
func (r *Registry) Register(a Adapter) {
	if a == nil {
		return
	}
	r.adapters[a.ID()] = a
	r.order = append(r.order, a.ID())
}

// Get returns the adapter with the given id.
func (r *Registry) Get(id string) (Adapter, bool) {
	a, ok := r.adapters[id]
	return a, ok
}

// List returns all registered adapters in registration order.
func (r *Registry) List() []Adapter {
	out := make([]Adapter, 0, len(r.order))
	for _, id := range r.order {
		if a, ok := r.adapters[id]; ok {
			out = append(out, a)
		}
	}
	return out
}

// IDs returns adapter IDs in registration order.
func (r *Registry) IDs() []string { return append([]string(nil), r.order...) }
