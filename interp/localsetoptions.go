// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Shell options that are the *call's* rather than the shell's.
//
// The shell that keys the scoping on the definition form has two function
// definition forms and they are two different scoping constructs. `typeset`
// already knew that here and the trap table learned it in #2345's fifth pass;
// this is the third table the same word carries, and the option table is the
// substrate's rather than any dialect's, which is why the save lives beside
// the traps' instead of behind AtEveryFunctionCall. That seam is a dialect's
// and cannot see how the function was written.
//
// # Restored, not reset
//
// The distinction is the whole of the model and it is not the one the trap
// table has. A `function` call there is handed an **empty** trap table; here
// it is handed the caller's table exactly as it stands, and the table is put
// back at the return. Measured 2026-09-16 on AT&T 93u+ 2012-08-01, a script
// file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin from /dev/null,
// in both directions so that neither answer can be an accident of which way a
// default points:
//
//	set -o noglob; function g { case $- in (*f*) echo body-on;;
//	  (*) echo body-off;; esac; }; g            body-on
//	set +o trackall; function g { [[ -o trackall ]] &&
//	  echo body-on || echo body-off; }; g       body-off
//
// One option the caller turned on that starts off, one it turned off that
// starts on, and the body read each as the caller left it. A table emptied or
// reset to the shell's defaults on the way in answers the opposite to both.
//
// This is what a probe that only *writes* in the body and then reads the
// caller cannot see: `function g { set -o noglob; }; g` is `off` afterwards
// under a restore and under a reset alike. See
// Semantics.FunctionLocalOptions.

// keywordCallScopesOptions reports whether this call is one whose option
// table is its own.
//
// **Read rather than asked**, which is the shape keywordGatesALocalScope
// already has next door and wants its reason here too. Every function call in
// the shell comes through the save, and most of them never move an option —
// asking at the entry would put the unanswered-axis diagnostic in front of
// every call a bare Semantics ever makes, over a question that call never
// puts.
func (r *Runner) keywordCallScopesOptions(sc *scope) bool {
	return sc.keyword &&
		r.sem().FunctionLocalOptions == OptionsGoBackAtTheReturnOfAKeywordFunction
}

// saveTheOptionTable takes the table as this call found it.
//
// The whole roster and not a chosen few, walked through the same lookup a
// `set -o` operand walks: an option a dialect declares tomorrow is scoped by
// this the day it is declared, where a list of fields written out here would
// have to be remembered. Measured a name at a time over nineteen of that
// shell's own names — allexport, errexit, noglob, nounset, noclobber, xtrace,
// verbose, notify, monitor, keyword, trackall, markdirs, nolog, bgnice,
// ignoreeof, emacs, vi, gmacs and pipefail — with the caller setting each and
// the body clearing it: every one came back at the return.
//
// Two kinds are left out, and neither is an omission: a name the shell will
// not move in either direction (AddImmovableSetOptions) cannot have been
// moved by the body, and a name it takes with nothing behind it
// (AddInertSetOptions) has no state to put back. Both would report a refusal
// on the way out if the restore asked for them.
func (r *Runner) saveTheOptionTable(sc *scope) {
	if !r.keywordCallScopesOptions(sc) {
		return
	}
	names := r.listedOptionNames()
	saved := make(map[string]bool, len(names))
	for _, name := range names {
		if r.immovableName(name) || r.inertOptions[name] {
			continue
		}
		o, ok := r.lookupSetOption(name)
		if !ok {
			continue
		}
		saved[name] = o.state(r)
	}
	sc.savedOptions = saved
}

// restoreTheOptionTable puts it back as the call unwinds.
//
// In the listing's order rather than the map's, so that a return which does
// move several names moves them the same way twice — the same reason
// sortedTrapNames orders its own restore.
//
// It is **not** load-bearing, and that is worth writing down because one pair
// looks as though it should be: `vi` and `emacs` are two names over one
// state, so a walk could plausibly leave the mode the body chose standing.
// Measured by reversing this walk — it does not, in either direction,
// because turning a mode off only moves anything when that mode is the
// selected one. A mutant that walks the map instead survives every row here,
// which is the claim this comment is making and not an oversight.
//
// Only where the state actually differs. A call that moved nothing is the
// overwhelmingly common one, and several of these names do real work when
// they are written — `posix` moves a dozen axes, `monitor` asks the operating
// system for a terminal — so asking each of them for the state it already
// holds would put that work on the way out of every `function` call in the
// shell.
func (r *Runner) restoreTheOptionTable(sc *scope) {
	if sc.savedOptions == nil {
		return
	}
	for _, name := range r.listedOptionNames() {
		want, saved := sc.savedOptions[name]
		if !saved {
			continue
		}
		o, ok := r.lookupSetOption(name)
		if !ok || o.state(r) == want {
			continue
		}
		r.putOptionBack(name, o, want)
	}
	sc.savedOptions = nil
}

// putOptionBack writes one option's saved state without going through the
// refusals a `set` operand meets.
//
// Nothing here is a request a script made, so nothing here can be refused:
// the state being written is one this shell was in a moment ago, and a
// complaint on the way out of a function call would name a word no script
// wrote. That is also why the table's entries are reached directly rather
// than through setOption, whose job is to decide what to say about an operand.
func (r *Runner) putOptionBack(name string, o setOption, want bool) {
	if name == "pipefail" {
		// The one name whose *existence* is an axis rather than a table
		// entry, so its row carries a read and no write — see setOption,
		// which asks the axis instead. The question was already put and
		// answered when the body moved the state, and putting it again here
		// would ask a dialect about a word this call is not running.
		r.pipefail = want
		return
	}
	switch {
	case o.try != nil:
		o.try(r, want, name)
	case o.apply != nil:
		o.apply(r, want)
	}
	// A name with neither is one the dialect declared and this shell knows
	// nothing else about: its state is the constant the table hands back, so
	// it cannot have moved and there is nothing to put back. A *recorded*
	// name is not that case — the recording is its state and apply is what
	// writes it — so those are restored like any other.
}

// suspendWhatAKeywordCallStartsWithout turns off the two options the shell
// with this rule does not hand a keyword-defined body.
//
// Beside the save above and gated on the same question, which is deliberate
// and is the one thing about this that is not free to be arranged: what is
// turned off here is put back by the restore at the return, so a shell that
// suspended without saving would lose the caller's `-e` for the rest of the
// run. The two axes are separate facts — a restore is not a reset, and this
// names two options rather than a table — and the one column that has either
// has both. See Semantics.KeywordFunctionSuspendsErrexitAndXtrace for what
// was measured, letter by letter.
//
// The letters are not `-e` and `-x` because nothing here is a letter: the
// option table is keyed by name, and the letter for a name is a dialect's
// spelling of it. `putOptionBack` is what writes them for the reason it
// writes the restore — nothing here is a request a script made, so nothing
// here can be refused, and a complaint on the way *into* a function call
// would name a word no script wrote.
func (r *Runner) suspendWhatAKeywordCallStartsWithout(sc *scope) {
	if !r.keywordCallScopesOptions(sc) {
		return
	}
	if r.sem().KeywordFunctionSuspendsErrexitAndXtrace != Yes {
		return
	}
	for _, name := range [...]string{"errexit", "xtrace"} {
		o, ok := r.lookupSetOption(name)
		if !ok || !o.state(r) {
			continue
		}
		r.putOptionBack(name, o, false)
	}
}
