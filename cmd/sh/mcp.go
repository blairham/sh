// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/mcpboot"
)

// `sh -mcp`: the Model Context Protocol, served on standard input and output.
//
// A coding agent or assistant launches this as a subprocess and speaks
// JSON-RPC to it a line at a time. What it gets is five tools that start a
// command, read what it has written, wait for it, kill it and let it go —
// every one of them through the same gate a script's command passes.
//
// It is the fourth route onto the gate and event seam from this binary, and
// the one that reaches the largest client population: MCP is spoken by most
// coding agents, where ACP is essentially one editor family.
//
// The reason it is worth a second protocol at all is the invocation rather
// than the protocol. #1334 records that a policy cannot reach a coding agent,
// because the agent invokes `$SHELL -c` and there is nowhere on that line to
// put a flag. An MCP server is configured with a *user-controlled* command
// line:
//
//	claude mcp add shell -- <path>/libexec/sh/sh -mcp -dialect bash -policy ~/.config/agent.policy
//
// so the policy rides along.
//
// The revision is `2026-07-28`, with the `2025-11-25` handshake served beside
// it for the clients that still open with one; internal/mcp pins down which is
// which, docs/design/mcp.md argues the subset a shell can honestly serve, and
// both record the measurement that put the second one there.

// serveMCP runs the protocol until the client closes our input, and returns
// the status to exit with.
//
// in and out are the connection, and they are passed rather than reached for,
// for the reason serveACP's are: a route that opens its own streams is a route
// no test can be on the other end of, and `-acp -policy p` was wrong for as
// long as it was untestable (#1335).
func serveMCP(sh driver.Shell, rest []string, in io.Reader, out io.Writer) int {
	if len(rest) > 0 {
		// There is no invocation to read here: a tool call names its own
		// command and its own directory. Operands would be silently ignored,
		// which is the failure mode where somebody's `-c` never runs and
		// nothing says why.
		fmt.Fprintf(os.Stderr, "sh: -mcp takes no operands: %v\n", rest)
		return exitFailure
	}
	sh.Stdin, sh.Stdout = in, out
	sh.Version = version
	return mcpboot.ServeAs("sh")(sh)
}
