// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"
)

// `sh -acp`: the Agent Client Protocol, served on standard input and output.
//
// A client — an editor, or another program — launches this as a subprocess and
// speaks JSON-RPC to it a line at a time. What it gets is a shell per session,
// gated: every command the shell is about to run, every file it is about to
// write and every signal it is about to send is put to the client as a
// permission request before it happens, and everything it does is reported as
// it happens.
//
// It is the third route onto the gate and event seam from this binary, and the
// first one that is not a debug surface. `-trace-events` prints the stream and
// `-deny` refuses a path; this hands both to somebody who can answer.
//
// The version is what the protocol calls 1, over stdio, which
// docs/design/acp.md pins down along with the subset a shell can honestly
// serve.

// version is what this binary calls itself to a client. It is not a release
// number — this repository has none yet — and saying so is better than
// inventing one that will be wrong.
const version = "0.0.0-dev"

// serveACP runs the protocol until the client closes our input, and returns
// the status to exit with.
func serveACP(sh driver.Shell, rest []string) int {
	if len(rest) > 0 {
		// There is no invocation to read here: a session's directory and its
		// program both arrive as messages. Operands would be silently
		// ignored, which is the failure mode where somebody's `-c` never
		// runs and nothing says why.
		fmt.Fprintf(os.Stderr, "sh: -acp takes no operands: %v\n", rest)
		return exitFailure
	}
	agent := acp.NewAgent(sh, acp.Implementation{
		Name:    "sh",
		Title:   "sh",
		Version: version,
	})
	ctx := context.Background()
	// Standard output is the protocol's and nothing else may be written
	// there — the shell's own output goes back as session updates, on writers
	// each session gives its runner. Standard error stays ours: the protocol
	// says an agent may log there and that a client may ignore it.
	err := agent.Serve(ctx, os.Stdin, os.Stdout)
	// Every session's EXIT trap fires once, as the end of a script does,
	// before this process goes.
	agent.Close(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sh: acp: %v\n", err)
		return exitFailure
	}
	return 0
}
