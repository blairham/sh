// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package termhost

import (
	"context"
	"io"
	"strings"

	"github.com/blairham/sh/driver"
)

// Interpreter runs one command line on a shell built like this one.
//
// The Shell is copied and three fields are written over — where its output
// goes, where it starts, and what it starts with — and everything else comes
// across untouched, which is the part that matters: the dialect, the gate and
// the event sink are the session's own, so a line a peer asks us to run is
// gated and recorded exactly as a line a person typed would be. Building a
// fresh Shell here instead would be a second shell with none of that, wearing
// the same name.
//
// Output and diagnostics go to the same writer because a terminal has one.
//
// It lives here rather than beside either front end because both want it and
// neither owns it: `--acp-connect` wires it into an ACP client, `--mcp` wires
// it into an MCP server, and a second copy is how the KeepProcess line below
// comes to be in one of them and not the other.
func Interpreter(sh driver.Shell) func(context.Context, Command, io.Writer) int {
	return func(ctx context.Context, cmd Command, out io.Writer) int {
		run := sh
		run.Context = ctx
		run.Stdin = strings.NewReader("")
		run.Stdout, run.Stderr = out, out
		if cmd.Dir != "" {
			run.Dir = cmd.Dir
		}
		run.Env = cmd.Env
		// The one thing a shell binary wants that this route must not have.
		// The field is negative so that the zero value suits a binary being a
		// shell — and here the zero value is exactly backwards: an `exec`
		// inside the peer's line would replace *this* process, which is the
		// one serving the connection the peer is talking on, and which is
		// the process that *is* the boundary. Every gate consultation and
		// every audit record for the session comes from it, so a replaced one
		// is not a crashed session but a program of the peer's choosing
		// holding this process's descriptors with nothing left to consult
		// (#1795).
		//
		// The line still gets its `exec`, as a child — which is what every
		// other embedder of interp gets by default — and the session survives
		// it.
		run.KeepProcess = true
		return driver.RunCommand(run, cmd.Line, nil)
	}
}
