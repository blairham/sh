// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acpboot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/interp"
)

// The prompt from #6937: an instruction and the four lines it refers to. Read a
// line at a time it is five turns, the first of which asks about commands it
// has not been given.
const fourCommands = "Run each of these four commands and report the results:\n" +
	"1. echo one\n" +
	"2. echo two\n" +
	"3. echo three\n" +
	"4. echo four\n"

// scriptedAgent is an agent this test controls completely, in the shape of
// internal/acpcheck's: it answers the handshake, opens one session, and
// writes down the text of every session/prompt it is sent. It is written
// against the wire rather than built on internal/acp's agent, so that what it
// records is what crossed the pipe and not what one half of a package agreed
// with the other half about.
type scriptedAgent struct {
	mu      sync.Mutex
	prompts []string
}

func (a *scriptedAgent) serve(conn net.Conn) {
	in := bufio.NewScanner(conn)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	send := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = conn.Write(append(b, '\n'))
	}
	for in.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &m) != nil || m.Method == "" {
			continue
		}
		switch m.Method {
		case "initialize":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{
				"protocolVersion":   1,
				"agentCapabilities": map[string]any{"loadSession": false, "promptCapabilities": map[string]any{}},
				"authMethods":       []any{},
				"agentInfo":         map[string]any{"name": "scripted", "version": "0"},
			}})
		case "session/new":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"sessionId": "s1"}})
		case "session/prompt":
			var p struct {
				Prompt []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"prompt"`
			}
			_ = json.Unmarshal(m.Params, &p)
			var text strings.Builder
			for _, b := range p.Prompt {
				text.WriteString(b.Text)
			}
			a.mu.Lock()
			a.prompts = append(a.prompts, text.String())
			a.mu.Unlock()
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"stopReason": "end_turn"}})
		default:
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32601, "message": m.Method}})
		}
	}
}

// promptsSent runs talk against the scripted agent with the given source and
// returns every prompt the agent received.
func promptsSent(t *testing.T, next prompts) []string {
	t.Helper()
	a, b := net.Pipe()
	agent := &scriptedAgent{}
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); agent.serve(a) }()
	client := &acp.Client{Info: acp.Implementation{Name: "sh", Version: "test"}}
	client.Connect(b, b)
	go func() { defer wg.Done(); _ = client.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = a.Close()
		_ = b.Close()
		wg.Wait()
	})
	if code := talk("sh", "-", ctx, client, "", next); code != 0 {
		t.Fatalf("talk exited %d", code)
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return slices.Clone(agent.prompts)
}

// The whole of #6937 in one comparison. The same text, read the two ways: a
// line at a time it reaches the agent as five turns, and as -acp-prompt reads
// it, as one turn holding every line. The first half is the control — it is
// the behavior the issue reported, and it proves the agent here counts turns
// rather than agreeing with anything it is shown.
func TestAWholePromptIsOneTurn(t *testing.T) {
	perLine := promptsSent(t, lines(bufio.NewScanner(strings.NewReader(fourCommands))))
	if len(perLine) != 5 {
		t.Fatalf("line at a time: the agent got %d turns, want 5: %q", len(perLine), perLine)
	}

	text, err := readPrompt(t.Context(), boundary.Boundary{}, "-", strings.NewReader(fourCommands))
	if err != nil {
		t.Fatal(err)
	}
	whole := promptsSent(t, whole(text))
	want := strings.TrimRight(fourCommands, "\n")
	if len(whole) != 1 || whole[0] != want {
		t.Fatalf("-acp-prompt: the agent got %q, want one turn holding %q", whole, want)
	}
}

// A prompt file is read whole, from a file or from standard input; trailing
// newlines go, as the line reader drops the one each line ends on, and
// everything inside stays — blank lines included, since a paragraph break is
// part of what was written.
func TestReadPrompt(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "task.md")
	if err := os.WriteFile(file, []byte("first\n\nsecond\r\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readPrompt(t.Context(), boundary.Boundary{}, file, strings.NewReader("not this"))
	if err != nil || got != "first\n\nsecond" {
		t.Errorf("from a file: %q, %v", got, err)
	}
	got, err = readPrompt(t.Context(), boundary.Boundary{}, "-", strings.NewReader("from\nstdin\n"))
	if err != nil || got != "from\nstdin" {
		t.Errorf("from standard input: %q, %v", got, err)
	}

	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, []byte(" \n\n\t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrompt(t.Context(), boundary.Boundary{}, empty, nil); err == nil {
		t.Error("a file of whitespace was accepted as a prompt")
	}
	if _, err := readPrompt(t.Context(), boundary.Boundary{}, "-", strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "standard input") {
		t.Errorf("an empty standard input: %v, want a refusal naming it", err)
	}
	if _, err := readPrompt(t.Context(), boundary.Boundary{}, filepath.Join(dir, "absent"), nil); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file: %v, want not-exist", err)
	}
}

// The prompt file goes through the same gate as the agent's own reads: its
// contents are handed to the agent, so a policy that refuses the file must
// refuse it here, or the prompt would be a way round the policy. The control
// is the same file with no gate, which reads.
func TestAPromptFileIsReadThroughTheGate(t *testing.T) {
	file := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(file, []byte("the secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readPrompt(t.Context(), boundary.Boundary{}, file, nil); err != nil || got != "the secret" {
		t.Fatalf("ungated: %q, %v", got, err)
	}
	asked := 0
	deny := boundary.Boundary{Gate: interp.GateFunc(func(context.Context, interp.Action) interp.Decision {
		asked++
		return interp.Deny
	})}
	got, err := readPrompt(t.Context(), deny, file, nil)
	if !errors.Is(err, boundary.ErrRefused) || got != "" {
		t.Errorf("denied: %q, %v, want a refusal", got, err)
	}
	if asked == 0 {
		t.Error("the gate was never asked")
	}
	// Standard input is a descriptor, not a path: the same gate is not asked.
	asked = 0
	if got, err := readPrompt(t.Context(), deny, "-", strings.NewReader("piped\n")); err != nil || got != "piped" || asked != 0 {
		t.Errorf("standard input under a denying gate: %q, %v, asked %d", got, err, asked)
	}
}

// A prompt read from standard input has used it up, so nobody is left there to
// answer a permission request — even on a terminal, where a person typed the
// prompt and ended it. Nobody to ask is a denial, which is the ladder's last
// rung. A prompt from a file leaves the terminal to the person, as before.
func TestAPromptFromStandardInputLeavesNobodyToAsk(t *testing.T) {
	control, term, err := pty.Open()
	if err != nil {
		if errors.Is(err, pty.ErrUnsupported) {
			t.Skip("no pseudo-terminal on this platform")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = term.Close() })

	for _, tc := range []struct {
		prompt string
		want   bool
	}{
		{"", true},
		{"task.md", true},
		{"-", false},
	} {
		if got := askable(tc.prompt, term, term); got != tc.want {
			t.Errorf("prompt %q on a terminal: askable = %v, want %v", tc.prompt, got, tc.want)
		}
	}
}
