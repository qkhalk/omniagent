package adapter

import (
        "context"
        "errors"
        "strings"
)

// ClaudeAdapter wraps the `claude` CLI in agentic print mode.
//
//   claude -p --output-format stream-json --input-format stream --verbose
//
// Reads the prompt from stdin (one JSON message per line).
type ClaudeAdapter struct{ baseAdapter }

// NewClaudeAdapter builds a ClaudeAdapter.
func NewClaudeAdapter(bin string, env []string) *ClaudeAdapter {
        return &ClaudeAdapter{baseAdapter{
                id: "claude", display: "Claude Code",
                bin:  bin,
                args: []string{"-p", "--output-format", "stream-json", "--input-format", "stream", "--verbose"},
                env:  env,
        }}
}

// ID returns "claude".
func (a *ClaudeAdapter) ID() string { return a.id }

// DisplayName returns the human label.
func (a *ClaudeAdapter) DisplayName() string { return a.display }

// Available checks the PATH.
func (a *ClaudeAdapter) Available(ctx context.Context) bool { return a.available(ctx) }

// Run executes the subprocess.
func (a *ClaudeAdapter) Run(ctx context.Context, spec Spec, w EventWriter) error {
        // Wrap the prompt as a stream-JSON user message and prepend any model override.
        args := []string{}
        if spec.Model != "" {
                args = append(args, "--model", spec.Model)
        }
        oldArgs := a.args
        a.args = append(args, a.args...)
        defer func() { a.args = oldArgs }()

        return a.run(ctx, spec, w, func(line string) []Event {
                if ev, ok := jsonEvent(line); ok {
                        if ev.Type == "text" && ev.Text == "" {
                                return nil
                        }
                        return []Event{ev}
                }
                ev := plainLine(line)
                if ev.Type == "" {
                        return nil
                }
                return []Event{ev}
        })
}

// CodexAdapter wraps the OpenAI `codex` CLI in exec mode.
//
//   codex exec --json -
//
// Reads the prompt from stdin as plain text.
type CodexAdapter struct{ baseAdapter }

// NewCodexAdapter builds a CodexAdapter.
func NewCodexAdapter(bin string, env []string) *CodexAdapter {
        return &CodexAdapter{baseAdapter{
                id: "codex", display: "OpenAI Codex",
                bin:  bin,
                args: []string{"exec", "--json"},
                env:  env,
        }}
}

func (a *CodexAdapter) ID() string         { return a.id }
func (a *CodexAdapter) DisplayName() string { return a.display }
func (a *CodexAdapter) Available(ctx context.Context) bool { return a.available(ctx) }

func (a *CodexAdapter) Run(ctx context.Context, spec Spec, w EventWriter) error {
        if spec.Model != "" {
                oldArgs := a.args
                a.args = append([]string{"exec", "--json", "-m", spec.Model}, []string{}...)
                defer func() { a.args = oldArgs }()
        }
        return a.run(ctx, spec, w, func(line string) []Event {
                if ev, ok := jsonEvent(line); ok {
                        if ev.Type == "text" && ev.Text == "" {
                                return nil
                        }
                        return []Event{ev}
                }
                ev := plainLine(line)
                if ev.Type == "" {
                        return nil
                }
                return []Event{ev}
        })
}

// GeminiAdapter wraps Google's `gemini` CLI.
//
//   gemini -p --output-format stream-json
//
type GeminiAdapter struct{ baseAdapter }

// NewGeminiAdapter builds a GeminiAdapter.
func NewGeminiAdapter(bin string, env []string) *GeminiAdapter {
        return &GeminiAdapter{baseAdapter{
                id: "gemini", display: "Gemini CLI",
                bin:  bin,
                args: []string{"-p", "--output-format", "stream-json"},
                env:  env,
        }}
}

func (a *GeminiAdapter) ID() string         { return a.id }
func (a *GeminiAdapter) DisplayName() string { return a.display }
func (a *GeminiAdapter) Available(ctx context.Context) bool { return a.available(ctx) }

func (a *GeminiAdapter) Run(ctx context.Context, spec Spec, w EventWriter) error {
        return a.run(ctx, spec, w, func(line string) []Event {
                if ev, ok := jsonEvent(line); ok {
                        if ev.Type == "text" && ev.Text == "" {
                                return nil
                        }
                        return []Event{ev}
                }
                ev := plainLine(line)
                if ev.Type == "" {
                        return nil
                }
                return []Event{ev}
        })
}

// AntigravityAdapter wraps the Google Antigravity CLI.
//
//   antigravity run --json --prompt -
//
// Antigravity's streaming protocol emits one JSON object per line; we reuse
// the same jsonEvent parser used for Claude.
type AntigravityAdapter struct{ baseAdapter }

// NewAntigravityAdapter builds an AntigravityAdapter.
func NewAntigravityAdapter(bin string, env []string) *AntigravityAdapter {
        return &AntigravityAdapter{baseAdapter{
                id: "antigravity", display: "Antigravity CLI",
                bin:  bin,
                args: []string{"run", "--json", "--prompt", "-"},
                env:  env,
        }}
}

func (a *AntigravityAdapter) ID() string         { return a.id }
func (a *AntigravityAdapter) DisplayName() string { return a.display }
func (a *AntigravityAdapter) Available(ctx context.Context) bool { return a.available(ctx) }

func (a *AntigravityAdapter) Run(ctx context.Context, spec Spec, w EventWriter) error {
        return a.run(ctx, spec, w, func(line string) []Event {
                if ev, ok := jsonEvent(line); ok {
                        if ev.Type == "text" && ev.Text == "" {
                                return nil
                        }
                        return []Event{ev}
                }
                ev := plainLine(line)
                if ev.Type == "" {
                        return nil
                }
                return []Event{ev}
        })
}

// ZCodeAdapter wraps the Z.ai ZCode CLI.
//
//   zcode chat --stream --input -
//
// Output is line-oriented; we emit each non-empty line as a text event
// unless it looks like a tool indicator.
type ZCodeAdapter struct{ baseAdapter }

// NewZCodeAdapter builds a ZCodeAdapter.
func NewZCodeAdapter(bin string, env []string) *ZCodeAdapter {
        return &ZCodeAdapter{baseAdapter{
                id: "zcode", display: "ZCode",
                bin:  bin,
                args: []string{"chat", "--stream", "--input", "-"},
                env:  env,
        }}
}

func (a *ZCodeAdapter) ID() string         { return a.id }
func (a *ZCodeAdapter) DisplayName() string { return a.display }
func (a *ZCodeAdapter) Available(ctx context.Context) bool { return a.available(ctx) }

func (a *ZCodeAdapter) Run(ctx context.Context, spec Spec, w EventWriter) error {
        return a.run(ctx, spec, w, func(line string) []Event {
                if ev, ok := jsonEvent(line); ok {
                        if ev.Type == "text" && ev.Text == "" {
                                return nil
                        }
                        return []Event{ev}
                }
                ev := plainLine(line)
                if ev.Type == "" {
                        return nil
                }
                return []Event{ev}
        })
}

// SafeJoin ensures the path stays within base. Used by adapters that take a
// project directory from the API layer.
func SafeJoin(base, rel string) (string, error) {
        rel = strings.TrimPrefix(rel, "/")
        if rel == "" {
                return base, nil
        }
        // reject traversal attempts
        if strings.Contains(rel, "..") {
                return "", errPathTraversal
        }
        return base + "/" + rel, nil
}

var errPathTraversal = errors.New("path traversal rejected")
