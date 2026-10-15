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
	r.storeOwnParameter(name, value, func() {
		r.fillOperandValue(name, value, r.markGetoptsOwnForAllexport)
	})
}

// storeOwnParameter runs store, which puts value into OPTARG or OPTIND as one
// of the shell's own writes rather than the script's, and holds the
// environment copy the store would otherwise refresh in the dialect that
// leaves it — the one mechanism every such write goes through, so that a
// second route cannot drop the hold (#6497).
//
// `getopts` is one such write. The other is the save, reset and restore a
// function call makes of OPTIND where the cursor is local to the call
// (Runner.localizeGetoptsCursor): measured 2026-10-15 on zsh 5.9.2,
// `export OPTIND=3; f() { printenv OPTIND; }; f` hands the child `3` while
// `$OPTIND` reads 1, and `export OPTIND=3; f() { OPTIND=7; }; f` hands a
// child `7` after the return has put `$OPTIND` back at 3. ksh93u+, whose
// `function`-form calls make the same save and restore, refreshes the copy
// both ways, as it does for `getopts`, so it is one axis and not two.
func (r *Runner) storeOwnParameter(name, value string, store func()) {
	held, holding := r.envCopyHeld[name]
	if !holding {
		held, holding = r.envCopyGetoptsLeaves(name, value)
	}
	store()
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

// envCopyGetoptsLeaves is the environment copy a store of value into name by
// `getopts` or a function call's save and restore would leave behind, and whether it leaves one: asked of the axis only
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
	} else if inherited, ok := r.inheritedValue(name); ok {
		// Never assigned since it arrived, so the entry a child is handed is
		// the one that came in, as environ's walk of the environment hands it
		// over: `OPTARG=pre` in the environment and `getopts x: o -x6` hands
		// a child `OPTARG=pre` in zsh 5.9.2, measured 2026-10-15.
		was = heldEnvCopy{value: inherited, present: true}
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
