// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acpboot"
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

// version is what this binary calls itself to a client. A build from a
// checkout says so rather than inventing a number that will be wrong; a
// release stamps the tag over it, which is why this is a var and not a const —
// `-X main.version=` can only write to a variable, and .goreleaser.yaml passes
// exactly that.
var version = "0.0.0-dev"

// serveACP runs the protocol until the client closes our input, and returns
// the status to exit with.
//
// in and out are the connection, and they are passed rather than reached for:
// they are the process's standard input and output when this is a binary, and
// they are a pipe when a test is the client. That is not tidiness — a route
// that opens its own streams is a route no test can be on the other end of,
// and `-acp -policy p` was wrong for as long as it was untestable (#1335).
func serveACP(sh driver.Shell, rest []string, in io.Reader, out io.Writer) int {
	if len(rest) > 0 {
		// There is no invocation to read here: a session's directory and its
		// program both arrive as messages. Operands would be silently
		// ignored, which is the failure mode where somebody's `-c` never
		// runs and nothing says why.
		fmt.Fprintf(os.Stderr, "sh: -acp takes no operands: %v\n", rest)
		return exitFailure
	}
	// The streams are this route's, not the Shell's: `-acp` is read by this
	// binary's own flag pass rather than by the front end, and a test is the
	// client on a pipe it made. Everything after that is the one
	// implementation both routes share — see internal/acpboot, and #2585 for
	// why a second copy here is the thing to avoid.
	sh.Stdin, sh.Stdout = in, out
	sh.Version = version
	return acpboot.ServeAs("sh")(sh)
}
