// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package acpboot supplies driver.Shell.ServeACP.
//
// It exists because of the import direction and nothing else: internal/acp
// takes a driver.Shell, so driver cannot import it back, and the hook has to
// be filled in by something that may import both. Every binary this repository
// ships points its ServeACP field here.
//
// One implementation rather than one per binary, deliberately. The duplication
// AGENTS.md endorses is each cmd building its own driver.Shell from its own
// dialect package — a central registry of dialects is what a binary meant to
// be liftable must not need. A copy of the *protocol server* in six mains is a
// different thing entirely, and this tree has the scar: a second helper that
// omits what the first carries is how a fix lands in one route and not the
// other.
package acpboot

import (
	"context"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"
)

// serveFailure is what a protocol that could not be served exits with. It
// matches cmd/sh's own exitFailure: the two routes onto this code must not
// disagree about what failure looks like.
const serveFailure = 2

// ServeAs builds the hook for a binary that announces itself by the given
// name.
//
// The name is a parameter and not `sh.Name`, which is the bug the dialect
// binaries found the day they got this flag: `driver` replaces Shell.Name with
// how the shell was *invoked*, on purpose, so that a diagnostic says `$0` the
// way every shell does — and a shell started by absolute path then announced
// `/tmp/acpzsh` to the client as its implementation name. A protocol's
// agentInfo is a stable identity rather than an invocation detail, and it is
// not somewhere to publish where the binary happens to live.
//
// The shell still names *itself* by `sh.Name` in a diagnostic below, because
// that one is about this run.
//
// The returned func runs the protocol on the shell's own streams until the
// client closes our input, and returns the status to exit with.
//
// The streams come off the Shell rather than being reached for, which is the
// property #1335 is about: a route that opens its own streams is a route no
// test can be on the other end of, and `-acp -policy p` was wrong for as long
// as it was untestable.
//
// What it announces is the *dialect*. A client that asked `zsh --acp` what is
// serving it should hear zsh — the shell whose semantics its sessions will
// have — rather than a name no part of the invocation mentioned.
func ServeAs(name string) func(driver.Shell) int {
	return func(sh driver.Shell) int { return serve(name, sh) }
}

// serve is ServeAs's body, with the announced name already settled.
func serve(name string, sh driver.Shell) int {
	agent := acp.NewAgent(sh, acp.Implementation{
		Name:    name,
		Title:   name,
		Version: sh.ReportedVersion(),
	})
	ctx := context.Background()
	// Standard output is the protocol's and nothing else may be written
	// there — the shell's own output goes back as session updates, on writers
	// each session gives its runner. Standard error stays ours: the protocol
	// says an agent may log there and that a client may ignore it.
	err := agent.Serve(ctx, sh.Stdin, sh.Stdout)
	// Every session's EXIT trap fires once, as the end of a script does,
	// before this process goes.
	agent.Close(ctx)
	if err != nil {
		// Named by the shell rather than by a constant: `zsh --acp` that
		// fails says zsh, which is the same rule every other diagnostic in
		// this front end follows.
		_, _ = sh.Stderr.Write([]byte(sh.Name + ": acp: " + err.Error() + "\n"))
		return serveFailure
	}
	return 0
}
