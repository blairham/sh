// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
)

// `--mcp`: the Model Context Protocol, on every binary this front end makes.
//
// It is `--acp`'s shape exactly, for a second protocol, and driver/acp.go
// carries the long version of every argument below. What is repeated here is
// only what is *measured* rather than inherited, because the measurement is
// about this spelling and not about that one.
//
// # The long form only, and that is measured
//
// A single-dash `-mcp` is not available and would not be safe: real bash reads
// it as `-m -c -p` — monitor, command string, privileged — and runs the
// command. Measured 2026-09-21 on macOS 25.6:
//
//	bash 5.3.20  `bash -mcp 'echo RAN'`   printed RAN
//	zsh 5.9.2    `zsh -mcp 'echo RAN'`    printed RAN
//
// So a single-dash spelling here would shadow working behavior rather than add
// a flag, which is the same finding `--acp` and `--policy` rest on. `cmd/sh`
// keeps its own single-dash `-mcp`, exactly as it keeps `-acp` and `-policy`:
// that binary is not a shell anyone's shebang names, so it has no bundle to
// collide with, and `sh -mcp` is the spelling #1338 records a person typing
// into `claude mcp add`.
//
// The other half is that the long form shadows nothing. Measured the same day,
// invoked `<shell> --mcp -c 'echo RAN'`. No shell ran the command:
//
//	bash 5.3.20   `--mcp: invalid option`     status 2
//	bash 3.2.57   `--mcp: invalid option`     status 2
//	zsh 5.9.2     `no such option: mcp`       status 1
//	ksh93u+ 2012  `mcp: bad option(s)`        status 2
//	dash          `Illegal option --`         status 2
//
// # Why it is a hook, why a missing hook refuses, and why it is served here
//
// All three are driver/acp.go's arguments unchanged: `internal/mcp` takes a
// Shell so driver cannot import it back; a binary that never wired the hook
// has no protocol to serve and says so rather than accepting the word
// silently; and the option is recorded during the option loop and acted on
// after the boundary is installed, so `zsh --policy p --mcp` governs every
// command a client asks for.
//
// That last one is the whole point of this flag existing. #1334 records that a
// policy cannot reach a coding agent because the agent invokes `$SHELL -c` and
// there is nowhere to put one; an MCP server is configured with a
// *user-controlled command line*, so the policy rides along.

// mcpOption reads `--mcp`.
//
// A flag and not a value, and an attached value is refused rather than
// ignored, for the reason `--acp=1` is: somebody expecting the word to mean
// something, and a shell that read it as a bare flag would serve a protocol on
// a spelling nobody agreed.
func mcpOption(word string, args []string, inv *invocation) (rest []string, matched bool, err error) {
	const attached = "--mcp="
	switch {
	case word == "--mcp":
		inv.mcp = true
		return args, true, nil
	case len(word) > len(attached) && word[:len(attached)] == attached:
		return nil, true, fmt.Errorf("--mcp takes no value")
	}
	return args, false, nil
}

// runMCP hands the process to the protocol server the binary supplied, and
// returns the status to exit with.
func (sh Shell) runMCP() int {
	if sh.ServeMCP == nil {
		// Refused, not ignored: a binary that never wired the hook has no
		// protocol to serve, and saying so is the whole difference between a
		// missing feature and a silent one.
		sh.errf("%s: --mcp: this binary does not serve the Model Context Protocol\n", sh.Name)
		return usageStatus
	}
	return sh.ServeMCP(sh)
}

// DevVersion is what a build from a checkout reports to a protocol client. Not
// a placeholder to be filled in — a client asking a development build what it
// is should be told that it is one.
const DevVersion = "0.0.0-dev"

// ReportedVersion is what to tell a protocol client when the build stamped
// nothing.
//
// One rule, read by every protocol front end. It was two — a `reported` in the
// package serving ACP, and the second one would have been written by copying
// it — and a dialect binary announcing a different version over one protocol
// than over the other is precisely the drift this tree keeps finding. It lives
// on Shell because Shell is where the field is.
func (sh Shell) ReportedVersion() string {
	if sh.Version == "" {
		return DevVersion
	}
	return sh.Version
}
