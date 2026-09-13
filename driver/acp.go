// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "fmt"

// `--acp`: the Agent Client Protocol, on every binary this front end makes.
//
// It was on `cmd/sh` alone, and that is the same drift `--policy` had before
// #1826: a capability reachable through the substrate's own driver and absent
// from the binaries a shebang, `chsh`, `login` and an editor's shell setting
// actually name. The instrument had the matching hole — `make acp` built
// `./cmd/sh` and drove it as `-dialect X`, so it graded one binary wearing
// three dialect hats and never the binaries people run. A green table said
// nothing about `zsh --acp`.
//
// # The long form only, and that is measured
//
// driver/sandbox.go records the measurement this rests on: real bash *accepts*
// `-acp` as `-a -c -p` and sets allexport, so a single-dash spelling here
// would shadow working behavior rather than add a flag. `cmd/sh` keeps its own
// single-dash `-acp`, exactly as it keeps `-policy`: that binary is not a
// shell anyone's shebang names, so it has no bundle to collide with.
//
// # Why it is a hook and not a call
//
// `internal/acp` imports this package — `NewAgent` takes a Shell — so `driver`
// cannot import it back. That is a cycle, and it is the reason the capability
// sat in `cmd/sh` rather than here in the first place.
//
// So this is the shape the tree already uses wherever the library may not do
// the thing itself: a func field that is nil in a library and filled in by the
// binary, exactly as `Runner.ReplaceProcess` and `Runner.DieBySignal` are. The
// front end owns *reading the option*, which is what "nothing outside driver/
// implements how a shell is invoked" requires, and the binary owns *serving
// the protocol*, which is what the import graph requires.
//
// # A missing hook refuses the option
//
// A dialect whose binary never set one answers `--acp` with an error rather
// than accepting the word and doing nothing. Silently accepting a flag and
// ignoring it is the exact failure this issue is about — `./bash -i` once
// answered `unknown option "-i"`, and the mirror of that is worse, because
// nothing says so.
//
// # Why it is read here and served after the gate
//
// A session's directory and its program both arrive as messages, so there is
// no invocation left to read once the protocol starts. What there *is* to read
// first is the boundary: `zsh --policy p --acp` must govern every session the
// client opens, exactly as it governs a script. So the option is recorded
// during the option loop and served after installSandbox has run — the same
// ordering, and for the same reason, as an operand being opened through the
// gate.
//
// # This does not make anything drive a shell over the protocol
//
// It makes the shipped dialect binaries capable of *serving* it. Whether an
// editor or an agent harness speaks it to them is the client's side.

// acpOption reads `--acp`.
//
// A flag and not a value, unlike `--policy`: there is nothing to name. An
// attached value is refused rather than ignored, because `--acp=1` is somebody
// expecting the word to mean something, and a shell that read it as a bare
// flag would serve a protocol on a spelling nobody agreed.
func acpOption(word string, args []string, inv *invocation) (rest []string, matched bool, err error) {
	const attached = "--acp="
	switch {
	case word == "--acp":
		inv.acp = true
		return args, true, nil
	case len(word) > len(attached) && word[:len(attached)] == attached:
		return nil, true, fmt.Errorf("--acp takes no value")
	}
	return args, false, nil
}

// runACP hands the process to the protocol server the binary supplied, and
// returns the status to exit with.
//
// The Shell it is given is the one the invocation built — gate, sink, streams
// and dialect — so a policy named on the same line governs every session the
// client opens.
func (sh Shell) runACP() int {
	if sh.ServeACP == nil {
		// Refused, not ignored. See the note above: a binary that never wired
		// the hook has no protocol to serve, and saying so is the whole
		// difference between a missing feature and a silent one.
		sh.errf("%s: --acp: this binary does not serve the Agent Client Protocol\n", sh.Name)
		return usageStatus
	}
	return sh.ServeACP(sh)
}
