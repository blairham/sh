// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// getoptsFillOwnParameter stores OPTARG or OPTIND as `getopts` writes them —
// the builtin's own two parameters, not its name operand — and, in the dialect
// whose store leaves an exported parameter's environment copy where it was,
// holds that copy for environ to hand a child. See
// Semantics.GetoptsLeavesTheEnvironmentCopyOfItsOwnParameters (#6475).
//
// The copy is the one a child was being handed *before* the first such
// store, and it is kept across every later one: `export OPTIND=1; getopts x:
// o -x6 -x7; getopts x: o -x6 -x7` still hands a child `OPTIND=1` in zsh
// 5.9.2. Any other store releases it (Runner.setVarAs), and so does taking
// the export off (Runner.exportedPut).
//
// The axis is asked only where the two readings would hand a child something
// different: the parameter is exported, a child would be handed an entry for
// it now, that entry is not the value being stored, and nothing holds it
// already — a hold is itself the earlier answer. A script that never exports
// OPTARG or OPTIND reaches no question.
func (r *Runner) getoptsFillOwnParameter(name, value string) {
	held, holding := r.envCopyHeld[name]
	if !holding {
		held, holding = r.envCopyGetoptsLeaves(name, value)
	}
	r.fillOperandValue(name, value, r.markGetoptsOwnForAllexport)
	if holding && r.isExported(name) {
		if r.envCopyHeld == nil {
			r.envCopyHeld = map[string]heldEnvCopy{}
		}
		r.envCopyHeld[name] = held
	}
}

// heldEnvCopy is what a child is handed for a name whose copy a store left
// behind: the value, or no entry at all where the name had none when the
// store was made — `export OPTARG=pre; unset OPTARG; getopts x: o -x6` hands
// a child no OPTARG in zsh 5.9.2, where `OPTARG=6` in place of the `getopts`
// hands it `OPTARG=6`.
type heldEnvCopy struct {
	value   string
	present bool
}

// envCopyGetoptsLeaves is the environment copy a `getopts` store of value into
// name would leave behind, and whether it leaves one: asked of the axis only
// once the store is known to be one the two readings part on.
func (r *Runner) envCopyGetoptsLeaves(name, value string) (heldEnvCopy, bool) {
	if !r.isExported(name) {
		return heldEnvCopy{}, false
	}
	var was heldEnvCopy
	if cur, set := r.varsLookup(name); set {
		if entry, ok := r.exportedScalarEntry(name, cur); ok {
			_, was.value, _ = strings.Cut(entry, "=")
			was.present = true
		}
	}
	if was.present && was.value == value {
		return heldEnvCopy{}, false
	}
	if !r.ask(r.sem().GetoptsLeavesTheEnvironmentCopyOfItsOwnParameters,
		"`getopts` refreshing the environment copy of an exported OPTARG or OPTIND") {
		return heldEnvCopy{}, false
	}
	return was, true
}

// releaseEnvCopy forgets the environment copy held for name, so a child is
// handed the value the shell has.
func (r *Runner) releaseEnvCopy(name string) {
	if r.envCopyHeld != nil {
		delete(r.envCopyHeld, name)
	}
}
