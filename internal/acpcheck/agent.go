// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acpcheck

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// The other direction: this instrument as an *agent*, so the shell can be
// graded as a client.
//
// `sh -acp-connect CMD` launches a coding agent and is the environment it runs
// inside — which is the half of the design no unit test can reach, because it
// needs a second process that asks the shell for things. A real coding agent
// needs an account, a network and a model, and #729 has been stuck on exactly
// that since the route was written; none of those are needed to ask a client
// to read a file.
//
// So this is a scripted agent. It asks for one thing, records what it was
// given or refused, and stops. What it asks for is the whole test: a file, and
// a command — the two things an agent under an editor does for itself, with
// nobody in a position to refuse.

// The environment a scripted agent reads. Passed through the shell, which
// inherits this process's environment and hands it to the agent it launches.
const (
	envAgentScript = "ACPCHECK_AGENT"
	envAgentOut    = "ACPCHECK_AGENT_OUT"
	envAgentTarget = "ACPCHECK_AGENT_TARGET"
)

// AgentReport is what a scripted agent writes down about how it was treated.
//
// Written to a file rather than to a stream, for a reason the protocol
// imposes: the agent's standard output is the connection and its standard
// error belongs to the shell's diagnostics. A file is the only channel out of
// an agent that is not also part of what is being measured.
type AgentReport struct {
	Script  string `json:"script"`
	Asked   string `json:"asked"`
	Got     string `json:"got"`
	Refused bool   `json:"refused"`
	Error   string `json:"error,omitempty"`
}

// RunAgent is the -as-agent mode: speak ACP as an agent until the client goes.
func RunAgent(script string) int {
	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()
	send := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = out.Write(append(b, '\n'))
		_ = out.Flush()
	}
	report := AgentReport{Script: script}
	defer func() {
		if path := os.Getenv(envAgentOut); path != "" {
			b, _ := json.Marshal(report)
			_ = os.WriteFile(path, b, 0o600)
		}
	}()

	// One reader, and requests we make are answered on it between the
	// client's own: an agent is a server and a caller on the same pipe.
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	nextID := 100
	// call sends a request to the client and reads until its answer arrives,
	// answering nothing else on the way — the scripted agent is asked for
	// nothing while it waits.
	call := func(method string, params any) (json.RawMessage, *RPCError) {
		nextID++
		id := nextID
		send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		for in.Scan() {
			var m struct {
				ID     *json.Number    `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *RPCError       `json:"error"`
			}
			if json.Unmarshal(in.Bytes(), &m) != nil || m.ID == nil {
				continue
			}
			if n, err := m.ID.Int64(); err == nil && int(n) == id {
				return m.Result, m.Error
			}
		}
		return nil, &RPCError{Message: "the client went away"}
	}

	for in.Scan() {
		var m struct {
			ID     json.Number     `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &m) != nil || m.Method == "" {
			continue
		}
		switch m.Method {
		case "initialize":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{
				"protocolVersion": 1,
				"agentCapabilities": map[string]any{
					"loadSession":        false,
					"promptCapabilities": map[string]any{},
				},
				"authMethods": []any{},
				"agentInfo":   map[string]any{"name": "acpcheck", "version": "0"},
			}})
		case "session/new":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"sessionId": "agent-1"}})
		case "session/prompt":
			var p struct {
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(m.Params, &p)
			target := os.Getenv(envAgentTarget)
			switch script {
			case "read":
				// What an agent under an editor would simply open.
				report.Asked = "fs/read_text_file " + target
				res, rpcErr := call("fs/read_text_file", map[string]any{
					"sessionId": p.SessionID, "path": target,
				})
				if rpcErr != nil {
					report.Refused, report.Error = true, rpcErr.Message
					break
				}
				var r struct {
					Content string `json:"content"`
				}
				_ = json.Unmarshal(res, &r)
				report.Got = r.Content
			case "run":
				// And what it would simply spawn.
				report.Asked = "terminal/create " + target
				res, rpcErr := call("terminal/create", map[string]any{
					"sessionId": p.SessionID, "command": target,
				})
				if rpcErr != nil {
					report.Refused, report.Error = true, rpcErr.Message
					break
				}
				var t struct {
					TerminalID string `json:"terminalId"`
				}
				_ = json.Unmarshal(res, &t)
				if _, rpcErr := call("terminal/wait_for_exit", map[string]any{
					"sessionId": p.SessionID, "terminalId": t.TerminalID,
				}); rpcErr != nil {
					report.Refused, report.Error = true, rpcErr.Message
					break
				}
				outRes, rpcErr := call("terminal/output", map[string]any{
					"sessionId": p.SessionID, "terminalId": t.TerminalID,
				})
				if rpcErr != nil {
					report.Refused, report.Error = true, rpcErr.Message
					break
				}
				var o struct {
					Output     string `json:"output"`
					ExitStatus *struct {
						ExitCode *int `json:"exitCode"`
					} `json:"exitStatus"`
				}
				_ = json.Unmarshal(outRes, &o)
				report.Got = o.Output
				_, _ = call("terminal/release", map[string]any{
					"sessionId": p.SessionID, "terminalId": t.TerminalID,
				})
			}
			// Say something back, so the turn reads like a turn.
			send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": p.SessionID,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content":       map[string]any{"type": "text", "text": agentSaid(report)},
				},
			}})
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"stopReason": "end_turn"}})
		default:
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{
				"code": -32601, "message": "acpcheck agent: " + m.Method,
			}})
		}
	}
	return 0
}

// agentSaid is the sentence the agent reports to the person at the other end.
func agentSaid(r AgentReport) string {
	if r.Refused {
		return fmt.Sprintf("the client refused %s: %s", r.Asked, r.Error)
	}
	return fmt.Sprintf("the client answered %s with %q", r.Asked, r.Got)
}
