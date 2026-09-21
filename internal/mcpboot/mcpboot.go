// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package mcpboot supplies driver.Shell.ServeMCP.
//
// It exists because of the import direction and nothing else: internal/mcp
// takes a driver.Shell, so driver cannot import it back, and the hook has to
// be filled in by something that may import both. Every binary this repository
// ships points its ServeMCP field here — one implementation rather than one
// per binary, for the reason internal/acpboot gives: a copy of a protocol
// server in six mains is how a fix lands in one route and not the others.
package mcpboot

import (
	"context"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/mcp"
)

// serveFailure is what a protocol that could not be served exits with. It
// matches internal/acpboot's and cmd/sh's exitFailure: the routes onto this
// code must not disagree about what failure looks like.
const serveFailure = 2

// ServeAs builds the hook for a binary that announces itself by the given
// name.
//
// The name is a parameter and not `sh.Name`, which is the bug the dialect
// binaries found the day they got `--acp`: `driver` replaces Shell.Name with
// how the shell was *invoked*, on purpose, so that a diagnostic says `$0` the
// way every shell does — and a shell started by absolute path then announced
// `/tmp/mcpzsh` to the client as its implementation name. A protocol's
// serverInfo is a stable identity rather than an invocation detail.
//
// What it announces is the *dialect*. A client that asked `zsh --mcp` what is
// serving it should hear zsh — the shell whose semantics its commands will
// have — rather than a name no part of the invocation mentioned.
//
// The returned func runs the protocol on the shell's own streams until the
// client closes our input, and returns the status to exit with. The streams
// come off the Shell rather than being reached for, which is the property
// #1335 is about: a route that opens its own streams is a route no test can be
// on the other end of.
func ServeAs(name string) func(driver.Shell) int {
	return func(sh driver.Shell) int { return serve(name, sh) }
}

func serve(name string, sh driver.Shell) int {
	server := mcp.NewServer(sh, mcp.Implementation{
		Name:    name,
		Title:   name,
		Version: sh.ReportedVersion(),
	})
	ctx := context.Background()
	// Standard output is the protocol's and nothing else may be written
	// there — what a command writes is held in its terminal and read back
	// through a tool. Standard error stays ours: the revision says a server
	// may log there and that a client may ignore it.
	err := server.Serve(ctx, sh.Stdin, sh.Stdout)
	// Nothing a client asked for outlives the connection it asked on.
	server.Close()
	if err != nil {
		// Named by the shell rather than by a constant: `zsh --mcp` that fails
		// says zsh, which is the rule every other diagnostic in this front end
		// follows.
		_, _ = sh.Stderr.Write([]byte(sh.Name + ": mcp: " + err.Error() + "\n"))
		return serveFailure
	}
	return 0
}
