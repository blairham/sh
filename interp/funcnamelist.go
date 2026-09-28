// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Whether a definition may name more than one function is a question about
// the **grammar** — `a b () { … }` is a definition or a syntax error at the
// `(`, and nothing about it is decided after the text is read — so it lives on
// syntax.Dialect and not on Semantics. One dialect spells it as an option a
// running script switches, which is why a runner has to be able to move it.
//
// That is the same combination [Runner.SetBarePatternGroups] and
// [Runner.SetShortFormBodyIsOneCommandOrNone] have, and the same consequences
// follow: the answer that decides is the one in force when the text is
// parsed, the dialect is copied and replaced rather than written through, and
// the front end's run loop reads the rest of the program with whatever it
// finds.
//
// Measured on `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)` — run `-f` over a script file under `set -n`,
// 2026-09-27, with the option moved on the line after an `emulate`:
//
//	                                on       off
//	a b () { echo "[$0]"; }         parses   refused
//	echo hi () { printf x; }        parses   refused
//	a b >out () { echo "[$0]"; }    parses   refused
//	a () { echo "[$0]"; }           parses   parses
//
// The last row is the control: one name is not this question, and an option
// that took the parenthesized form away altogether would have passed the other
// three alike.

// FunctionDefinitionTakesANameList reports whether a definition's header may
// carry more than one name.
func (r *Runner) FunctionDefinitionTakesANameList() bool {
	return r.lang().FunctionMultipleNames
}

// SetFunctionDefinitionTakesANameList moves it, for a dialect whose option
// namespace has a name for the reading.
//
// The dialect is copied and replaced rather than written through: the pointer
// is shared with every subshell cloned from this runner, and a script must not
// change the grammar of the shell that spawned it.
func (r *Runner) SetFunctionDefinitionTakesANameList(on bool) {
	d := r.dialect()
	if d.FunctionMultipleNames == on {
		return
	}
	d.FunctionMultipleNames = on
	r.Dialect = &d
}
