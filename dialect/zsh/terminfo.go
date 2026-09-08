// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// The `zsh/terminfo` and `zsh/termcap` modules: the terminal's capabilities,
// presented as two associations a script can read.
//
// The names are all that is here. What a capability *is* and what this shell
// can honestly say about one is [repl.TerminalCapabilities], for the reason
// every terminal fact in this tree sits there: it is a statement about the
// screen and about what this shell's own editor writes to it, and a dialect
// that owned it would be a dialect the substrate had to know about. This file
// is the two spellings — `$terminfo` keyed by terminfo's long names,
// `$termcap` by termcap's two-letter ones — over one set of measured values.
//
// # What this fixes, and it is not the parameter's own absence
//
// #1388. A plugin manager builds its entire color table behind one test:
//
//	if [[ -z $SOURCED && ( ${+terminfo} -eq 1 && -n ${terminfo[colors]} ) \
//	   || ( ${+termcap} -eq 1 && -n ${termcap[Co]} ) ]] { … }
//
// With neither parameter registered the test was false, the table was never
// filled in, and the formatter's `${ZI[col-$2]:-$1}` fell through to its own
// argument — which is the markup. So every message the shell printed during
// startup came out as `{error}Error{ehi}:{rst} …` rather than as colored
// text. #1369 recorded that line as the visible consequence of an arithmetic
// bug; it was this.
//
// Two halves of that test matter separately, and a partial table has to
// satisfy both. `${+terminfo}` is 1 because the parameter is *produced* — a
// produced association whose producer has entries is set, so registering the
// view is what makes the name exist. And `-n ${terminfo[colors]}` is
// satisfied because `colors` is one of the capabilities answered; the count
// comes from interp.TerminalColors, so what a theme is told matches what
// `%F{200}` will actually paint.
//
// # A key that is not answered refuses by name
//
// Thirteen capabilities are answered and every other name is refused —
// `terminfo[cnorm]: capability not implemented yet`, at the expansion that
// asked. Not empty, and the difference is the whole of what #1388 was:
// measured, real zsh's `$terminfo[colors]` is genuinely *absent* under
// `TERM=dumb`, so a caller reading an empty string cannot tell a terminal
// without the capability from a shell that never knew it. Answering empty
// from inside the parameter would be the same confusion with the parameter
// present, which is worse than the absence — the absence at least made
// `${+terminfo}` say no.
//
// A script that *asks* is answered rather than refused: `$+terminfo[cnorm]`
// is 0, and `${terminfo[cnorm]-}` is the script's own default. Both are
// exemptions in [interp.Runner.SetAbsentElements], and the first is not a
// nicety — swept across a real plugin tree, the set test is the commonest way
// these keys are touched, and a well-written theme reads `cnorm` only after
// `(( $+terminfo[civis] && $+terminfo[cnorm] ))` has told it there is
// something to read.
//
// # Both are readonly, and hidden with it
//
// Measured against zsh 5.9.2: `terminfo[colors]=9` is `read-only variable:
// terminfo`, `unset terminfo` the same, and `typeset -p terminfo` writes
// `typeset -Ar terminfo` — the bare name, with no values. So it is the pair
// `builtins` needs in parameter.go, and for the same two reasons: readonly
// because zsh says so, and hidden because readonly is an attribute, an
// attribute puts the name in the tables a listing walks, and a listing would
// otherwise write out the whole capability table as an assignment somebody
// could source back.
//
// # `echoti` and `echotc` are not here
//
// Each module names a builtin as well as a parameter — `zmodload -lF
// zsh/terminfo` is `+b:echoti` and `+p:terminfo` — and neither builtin is
// implemented. That is deliberate and it is the module rule in zmodload.go
// rather than an omission: a missing builtin refuses at its own call site, by
// name, on the line that ran it, so it never holds a module shut. The
// modules load now because their *parameters* are here, and a script that
// calls `echoti` finds out where it called it.

// registerTerminfoModules installs `$terminfo` and `$termcap`: two views over
// one capability table, each keyed by its own name system.
func registerTerminfoModules(r *interp.Runner) {
	registerCapabilityParameter(r, "terminfo", zshTerminfoView)
	registerCapabilityParameter(r, "termcap", zshTermcapView)
}

// registerCapabilityParameter installs one of them, with the three things a
// partial produced association needs: the producer, the refusal for the keys
// it has no answer for, and the readonly-and-hidden pair.
//
// One function for both because the two parameters differ in exactly one
// thing — which column of the capability table is the key — and writing them
// out twice is how the second one comes to be missing whatever the first one
// gains.
func registerCapabilityParameter(r *interp.Runner, name string, view func(*interp.Runner) interp.AssocArray) {
	r.SetDynamicAssoc(name, view)
	// The wording is this dialect's, because which shell has the module is
	// this dialect's; the mechanism is interp's, so a second dialect with a
	// partial table cannot arrive at a second answer. "Capability" rather
	// than "parameter": the parameter is there, and it is one key that is
	// not.
	r.SetAbsentElements(name, "capability not implemented yet")
	// Readonly rather than given a writer, which is zsh's own answer and the
	// same call `builtins` makes in parameter.go. A produced association with
	// neither would take an assignment into a stored table, and a stored
	// table is what a later read finds first — so one `terminfo[colors]=9`
	// would turn the view into a snapshot that never says it stopped
	// tracking.
	r.MarkReadonly(name)
	r.MarkHidden(name)
}

// zshTerminfoView is `$terminfo`: the capabilities this shell answers for,
// under terminfo's long names.
func zshTerminfoView(*interp.Runner) interp.AssocArray {
	caps := repl.TerminalCapabilities()
	out := make(interp.AssocArray, len(caps))
	for _, c := range caps {
		out[c.Terminfo] = c.Value
	}
	return out
}

// zshTermcapView is `$termcap`: the same capabilities under termcap's
// two-letter names.
//
// The same values and not a second measurement of them, which is measured
// rather than assumed: zsh's `$termcap` hands out terminfo's parameter
// language too — `${termcap[UP]}` is `\e[%p1%dA`, not termcap's own `%d`
// spelling — so one value serves both names.
func zshTermcapView(*interp.Runner) interp.AssocArray {
	caps := repl.TerminalCapabilities()
	out := make(interp.AssocArray, len(caps))
	for _, c := range caps {
		out[c.Termcap] = c.Value
	}
	return out
}
