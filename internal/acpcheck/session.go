// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acpcheck

import (
	"context"
	"encoding/json"
	"strings"
)

// The handshake and the two calls every row makes, named once.
//
// These are thin on purpose. A helper that hid the shape of a message would
// hide the thing being graded, so each one is the message written out with the
// one field a caller varies.

// Initialize performs the handshake and returns what the agent said about
// itself.
func (c *Client) Initialize(ctx context.Context) (Init, error) {
	res, err := c.Call(ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"fs":       map[string]any{"readTextFile": true, "writeTextFile": true},
			"terminal": true,
		},
	})
	if err != nil {
		return Init{}, err
	}
	var in Init
	err = json.Unmarshal(res, &in)
	return in, err
}

// Init is the initialize result, in the fields a client acts on.
type Init struct {
	ProtocolVersion int `json:"protocolVersion"`
	AgentInfo       struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"agentInfo"`
	AuthMethods []struct {
		ID string `json:"id"`
	} `json:"authMethods"`
	AgentCapabilities json.RawMessage `json:"agentCapabilities"`
}

// NewSession opens a session rooted at dir and returns its id.
func (c *Client) NewSession(ctx context.Context, dir string) (string, error) {
	res, err := c.Call(ctx, "session/new", map[string]any{
		"cwd": dir, "mcpServers": []any{},
	})
	if err != nil {
		return "", err
	}
	var s struct {
		SessionID string `json:"sessionId"`
	}
	err = json.Unmarshal(res, &s)
	return s.SessionID, err
}

// Prompt sends one prompt and returns why the turn stopped.
func (c *Client) Prompt(ctx context.Context, session, text string) (string, error) {
	res, err := c.Call(ctx, "session/prompt", map[string]any{
		"sessionId": session,
		"prompt":    []any{map[string]any{"type": "text", "text": text}},
	})
	if err != nil {
		return "", err
	}
	var r struct {
		StopReason string `json:"stopReason"`
	}
	err = json.Unmarshal(res, &r)
	return r.StopReason, err
}

// Cancel is the protocol's cancellation, which is a notification: the answer
// to a canceled turn is the turn's own response, with a stop reason.
func (c *Client) Cancel(session string) {
	c.Notify("session/cancel", map[string]any{"sessionId": session})
}

// Update is a session update decoded far enough to grade.
type Update struct {
	Kind       string // agent_message_chunk, tool_call, tool_call_update, …
	ToolCallID string
	Title      string
	ToolKind   string // execute, edit, read, other
	Status     string // pending, in_progress, completed, failed
	Text       string // an agent message chunk's text
	Stream     string // stdout or stderr, from the chunk's _meta
	Raw        json.RawMessage
}

// Updates returns every session update so far, decoded.
func (c *Client) Updates() []Update {
	var out []Update
	for _, n := range c.Notifications() {
		if n.Method != "session/update" {
			continue
		}
		var p struct {
			Update struct {
				Kind       string `json:"sessionUpdate"`
				ToolCallID string `json:"toolCallId"`
				Title      string `json:"title"`
				ToolKind   string `json:"kind"`
				Status     string `json:"status"`
				Content    struct {
					Text string `json:"text"`
				} `json:"content"`
				Meta map[string]string `json:"_meta"`
			} `json:"update"`
		}
		if json.Unmarshal(n.Params, &p) != nil {
			continue
		}
		u := Update{
			Kind:       p.Update.Kind,
			ToolCallID: p.Update.ToolCallID,
			Title:      p.Update.Title,
			ToolKind:   p.Update.ToolKind,
			Status:     p.Update.Status,
			Text:       p.Update.Content.Text,
			Raw:        n.Params,
		}
		for k, v := range p.Update.Meta {
			if strings.HasSuffix(k, "/stream") {
				u.Stream = v
			}
		}
		out = append(out, u)
	}
	return out
}

// Output is everything the shell printed, by stream.
//
// The streams are separated because merging them is how an instrument stops
// being able to tell a diagnostic from a result — the mistake the conformance
// harness made with CombinedOutput and had to be rescued from.
func (c *Client) Output(stream string) string {
	var b strings.Builder
	for _, u := range c.Updates() {
		if u.Kind == "agent_message_chunk" && u.Stream == stream {
			b.WriteString(u.Text)
		}
	}
	return b.String()
}

// ToolCalls returns the titles of the tool calls announced, in order.
//
// This is the count that matters to the argument in compare.go: one tool call
// is one action the shell was about to take that the client both saw and could
// have refused.
func (c *Client) ToolCalls() []Update {
	var out []Update
	for _, u := range c.Updates() {
		if u.Kind == "tool_call" {
			out = append(out, u)
		}
	}
	return out
}

// Status returns the last status reported for a tool call.
func (c *Client) Status(id string) string {
	status := ""
	for _, u := range c.Updates() {
		if u.ToolCallID == id && u.Status != "" {
			status = u.Status
		}
	}
	return status
}
