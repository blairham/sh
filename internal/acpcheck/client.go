// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package acpcheck is a real ACP client, used to grade a real ACP agent.
//
// The distinction from `internal/acp`'s own tests is the whole reason this
// exists. Those stand an agent up in this process and speak to it through a
// pipe made in the same test: they prove the two halves of one package agree
// with each other, which is worth proving and is not the question anyone asks
// about a protocol. The question is whether the binary we ship says what
// somebody else's client will understand — and the only way to answer that is
// to be somebody else's client: launch the executable, write JSON-RPC on its
// standard input, read its standard output, and believe nothing that is not on
// the wire.
//
// So nothing here imports the implementation. The message shapes are written
// out again by hand, from the protocol, because a client that shares the
// agent's types cannot catch the agent misnaming a field — both sides would be
// wrong together and the test would be green. That duplication is the point of
// the file and not an oversight in it.
package acpcheck

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Client is a connection to an agent running as a subprocess.
//
// One goroutine reads, everybody else writes under a lock, and requests are
// matched to their answers by id — the ordinary shape of a JSON-RPC client,
// written out rather than taken from a library so that the framing this
// grades is the framing the protocol describes.
type Client struct {
	cmd  *exec.Cmd
	in   io.WriteCloser
	done chan struct{}

	wmu sync.Mutex // serializes writes to the agent's standard input

	mu      sync.Mutex
	nextID  int
	pending map[int]chan reply
	notes   []Notification
	asks    []Ask
	impure  []string // lines on standard output that were not protocol
	stderr  strings.Builder

	// Answer decides what to reply to a permission request. It is called on
	// the reader goroutine, so it must not call back into the client.
	answer func(Ask) string

	// serve answers the requests an agent makes of a client other than
	// permission — fs/read_text_file and the terminal methods. Nil means
	// there is nothing to answer, which is correct for grading a shell:
	// this client withholds those capabilities. The tests fill it in,
	// because grading the *agent* mode needs a client that answers.
	serve func(method string, params json.RawMessage) (any, *RPCError)

	// trace, when set, is called with every line that crosses the
	// connection: "-->" for a line we wrote and "<--" for one we read. It is
	// how -wire prints a transcript that is the bytes rather than a
	// rendering of them.
	trace func(dir, line string)
}

// reply is one response envelope, either half of which may be empty.
type reply struct {
	Result json.RawMessage
	Err    *RPCError
}

// RPCError is the error object of a JSON-RPC response.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("%d: %s", e.Code, e.Message) }

// Notification is a `session/update` as it arrived, kept whole.
//
// Whole, because the interesting assertions are about fields a typed struct
// would drop: the `_meta` that says which stream a chunk came from, a
// `rawOutput` nobody has a type for yet. A grader that decodes into a struct
// can only ever check what the struct's author thought of.
type Notification struct {
	Method string
	Params json.RawMessage
}

// Ask is a `session/request_permission` the agent sent us, and what we said.
type Ask struct {
	ID       int
	Title    string   // the tool call's title, when the update carried one
	Kind     string   // execute, edit, read, …
	Options  []string // option ids, in the order offered
	Answered string   // the option id we replied with
}

// Options configure a connection.
type Options struct {
	// Args are the agent's arguments. `-acp` is not added for you: which
	// flags precede it is part of what some rows are testing.
	Args []string
	// Dir is the working directory to start the agent in.
	Dir string
	// Env, when non-nil, replaces the environment rather than adding to it.
	Env []string
	// Answer decides permission requests. A nil Answer allows once, which is
	// the answer that lets a row get far enough to be about something else.
	Answer func(Ask) string
	// Trace, when set, sees every line in both directions.
	Trace func(dir, line string)
	// Serve answers an agent's requests other than permission.
	Serve func(method string, params json.RawMessage) (any, *RPCError)
}

// Dial launches the agent and starts reading from it.
func Dial(bin string, opts Options) (*Client, error) {
	cmd := exec.Command(bin, opts.Args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	c := &Client{
		cmd:     cmd,
		in:      in,
		done:    make(chan struct{}),
		pending: map[int]chan reply{},
		answer:  opts.Answer,
		trace:   opts.Trace,
		serve:   opts.Serve,
	}
	if c.answer == nil {
		c.answer = func(Ask) string { return AllowOnce }
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Standard error is the agent's to log on and ours to ignore, says the
	// protocol — but a client that discards it has no idea why a handshake
	// timed out, so it is kept and reported with a failure.
	go func() { _, _ = io.Copy(&syncWriter{c: c}, errPipe) }()
	go c.read(out)
	return c, nil
}

// syncWriter collects the agent's standard error under the client's lock.
type syncWriter struct{ c *Client }

func (w *syncWriter) Write(p []byte) (int, error) {
	w.c.mu.Lock()
	defer w.c.mu.Unlock()
	w.c.stderr.Write(p)
	return len(p), nil
}

// The option ids this shell offers. They are the agent's to name and a client
// must echo one back exactly, which is itself one of the things graded: an id
// we invent must be refused rather than guessed at.
const (
	AllowOnce    = "allow-once"
	AllowAlways  = "allow-always"
	RejectOnce   = "reject-once"
	RejectAlways = "reject-always"
)

// read is the reader goroutine: one message per line, forever.
func (c *Client) read(out io.Reader) {
	defer close(c.done)
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			// A blank line is not a message, and it is not a violation
			// either: the framing is one message per line and an empty one
			// carries nothing. Skipped rather than recorded.
			continue
		}
		if c.trace != nil {
			c.trace("<--", line)
		}
		var m struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      *json.Number    `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
			Result  json.RawMessage `json:"result"`
			Error   *RPCError       `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &m); err != nil || m.JSONRPC != "2.0" {
			// This is the purity check, and it is made here because here is
			// where a client would break: anything on this stream that is
			// not a message is a message the client cannot parse, whether it
			// came from a stray print or from a child that inherited the
			// descriptor.
			c.mu.Lock()
			c.impure = append(c.impure, line)
			c.mu.Unlock()
			continue
		}
		switch {
		case m.Method == "session/request_permission" && m.ID != nil:
			c.handleAsk(*m.ID, m.Params)
		case m.Method != "" && m.ID != nil:
			// Any other request. An agent left waiting forever for an answer
			// is an agent that looks hung, so something is always sent back —
			// an error where there is nothing to answer with, which is what
			// a client that withholds the capability owes it.
			id, _ := m.ID.Int64()
			if c.serve == nil {
				c.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{
					"code": -32601, "message": "acpcheck client does not serve " + m.Method,
				}})
				continue
			}
			result, rpcErr := c.serve(m.Method, m.Params)
			if rpcErr != nil {
				c.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcErr})
				continue
			}
			c.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
		case m.Method != "" && m.ID == nil:
			c.mu.Lock()
			c.notes = append(c.notes, Notification{Method: m.Method, Params: m.Params})
			c.mu.Unlock()
		case m.ID != nil:
			id, _ := m.ID.Int64()
			c.mu.Lock()
			ch := c.pending[int(id)]
			delete(c.pending, int(id))
			c.mu.Unlock()
			if ch != nil {
				ch <- reply{Result: m.Result, Err: m.Error}
			}
		}
	}
}

// handleAsk answers a permission request, and records both halves.
func (c *Client) handleAsk(id json.Number, params json.RawMessage) {
	var p struct {
		ToolCall struct {
			ToolCallID string `json:"toolCallId"`
			Title      string `json:"title"`
			Kind       string `json:"kind"`
		} `json:"toolCall"`
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	_ = json.Unmarshal(params, &p)
	n, _ := id.Int64()
	ask := Ask{ID: int(n), Title: p.ToolCall.Title, Kind: p.ToolCall.Kind}
	for _, o := range p.Options {
		ask.Options = append(ask.Options, o.OptionID)
	}
	// The request carries only the tool call's id, so the title comes from
	// the `tool_call` update that announced it — which has always already
	// arrived, because a permission request is about something the agent has
	// told us it is about to do.
	if ask.Title == "" {
		ask.Title, ask.Kind = c.toolCallTitle(p.ToolCall.ToolCallID)
	}
	ask.Answered = c.answer(ask)
	c.mu.Lock()
	c.asks = append(c.asks, ask)
	c.mu.Unlock()
	c.write(map[string]any{
		"jsonrpc": "2.0", "id": n,
		"result": map[string]any{
			"outcome": map[string]any{"outcome": "selected", "optionId": ask.Answered},
		},
	})
}

// toolCallTitle finds what a tool call said it was, from the updates so far.
func (c *Client) toolCallTitle(id string) (title, kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, n := range c.notes {
		var p struct {
			Update struct {
				Kind       string `json:"sessionUpdate"`
				ToolCallID string `json:"toolCallId"`
				Title      string `json:"title"`
				ToolKind   string `json:"kind"`
			} `json:"update"`
		}
		if json.Unmarshal(n.Params, &p) != nil {
			continue
		}
		if p.Update.ToolCallID == id && p.Update.Title != "" {
			return p.Update.Title, p.Update.ToolKind
		}
	}
	return "", ""
}

func (c *Client) write(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.trace != nil {
		c.trace("-->", string(b))
	}
	_, _ = c.in.Write(append(b, '\n'))
}

// Call sends a request and waits for its answer.
//
// An agent that never answers is a failure of the kind this instrument exists
// to catch, so the wait is bounded by the context and a timeout is returned
// rather than hung on.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan reply, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	select {
	case r := <-ch:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Result, nil
	case <-c.done:
		return nil, fmt.Errorf("%s: the agent closed its output before answering", method)
	case <-ctx.Done():
		return nil, fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

// Notify sends a notification, which by definition has no answer.
func (c *Client) Notify(method string, params any) {
	c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// Notifications returns the session updates received so far.
func (c *Client) Notifications() []Notification {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Notification(nil), c.notes...)
}

// Asks returns the permission requests received so far, with our answers.
func (c *Client) Asks() []Ask {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Ask(nil), c.asks...)
}

// Impure returns the lines on standard output that were not protocol.
func (c *Client) Impure() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.impure...)
}

// Stderr returns what the agent logged.
func (c *Client) Stderr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stderr.String()
}

// Close shuts the connection down the way a client does: close the agent's
// input, which is the protocol's end-of-connection, and wait for it to go.
func (c *Client) Close() error {
	_ = c.in.Close()
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
	}
	err := c.cmd.Wait()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return fmt.Errorf("agent exited %d: %s", ee.ExitCode(), c.Stderr())
	}
	return err
}
