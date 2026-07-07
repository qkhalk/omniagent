package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// baseAdapter contains the shared bits every CLI adapter uses.
type baseAdapter struct {
	id      string
	display string
	bin     string
	args    []string // base args appended to every invocation
	env     []string // extra env (KEY=VAL)
}

// run spawns the subprocess and streams events. The parseLine callback
// converts each stdout line into zero or more Events.
func (b *baseAdapter) run(ctx context.Context, spec Spec, w EventWriter, parseLine func(line string) []Event) error {
	if b.bin == "" {
		return errors.New("adapter " + b.id + ": binary not configured")
	}
	args := append([]string{}, b.args...)
	args = append(args, spec.ExtraArgs...)

	cmd := exec.CommandContext(ctx, b.bin, args...)
	cmd.Dir = spec.ProjectDir
	cmd.Env = mergeEnv(b.env, spec.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// Write prompt to stdin then close.
	go func() {
		_, _ = io.WriteString(stdin, spec.Prompt)
		_ = stdin.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			for _, ev := range parseLine(line) {
				_ = w.WriteEvent(ev)
			}
		}
		if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
			_ = w.WriteEvent(Event{Type: "error", Text: "stdout scan: " + err.Error()})
		}
	}()

	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			_ = w.WriteEvent(Event{Type: "error", Text: sc.Text()})
		}
	}()

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-waitErr
		return ctx.Err()
	case err := <-waitErr:
		wg.Wait()
		_ = w.WriteEvent(Event{Type: "done"})
		return err
	}
}

// available checks if the binary is on PATH.
func (b *baseAdapter) available(ctx context.Context) bool {
	if b.bin == "" {
		return false
	}
	if _, err := exec.LookPath(b.bin); err == nil {
		return true
	}
	// try direct exec
	ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx2, b.bin, "--version")
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	return c.Run() == nil
}

func mergeEnv(base, extra []string) []string {
	out := append([]string{}, base...)
	out = append(out, extra...)
	return out
}

// --- helpers for line parsing ----------------------------------------------

// jsonEvent tries to parse a line as JSON and emit a single event.
func jsonEvent(line string) (Event, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "{") {
		return Event{}, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return Event{}, false
	}
	// Common shapes
	if t, _ := m["type"].(string); t != "" {
		ev := Event{Type: "text"}
		if t == "tool_use" || t == "tool" {
			ev.Type = "tool"
			ev.ToolName, _ = m["name"].(string)
			if in, ok := m["input"]; ok {
				if b, err := json.Marshal(in); err == nil {
					ev.ToolInput = string(b)
					if len(ev.ToolInput) > 240 {
						ev.ToolInput = ev.ToolInput[:240] + "..."
					}
				}
			}
		} else if text, _ := m["text"].(string); text != "" {
			ev.Text = text
		} else if c, ok := m["content"].([]any); ok {
			// Anthropic SDK style
			var sb strings.Builder
			for _, item := range c {
				if mm, ok := item.(map[string]any); ok {
					if tt, _ := mm["type"].(string); tt == "text" {
						if s, _ := mm["text"].(string); s != "" {
							sb.WriteString(s)
						}
					}
				}
			}
			ev.Text = sb.String()
		}
		return ev, true
	}
	return Event{}, false
}

// toolLineRe matches common "✏️ Edit: file.py" style lines from CLI output.
var toolLineRe = regexp.MustCompile(`^\s*(✏️|📖|📂|💻|🔍|📝|✅|❌|⚙️|🔧)\s+(\S.*)$`)

// plainLine emits a text event for an arbitrary stdout line.
func plainLine(line string) Event {
	trimmed := strings.TrimRight(line, "\r\n ")
	if trimmed == "" {
		return Event{}
	}
	if m := toolLineRe.FindStringSubmatch(trimmed); m != nil {
		return Event{Type: "tool", ToolName: m[1], ToolInput: m[2]}
	}
	return Event{Type: "text", Text: trimmed}
}
