// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// An **absent parameter** is one a dialect's module names and this shell has
// not got. Reading it refuses by name; it never reads as empty.
//
// This is the parameter half of the rule modules are loaded by, and it is the
// half that was missing. A missing *builtin* already refuses at its own call
// site — nothing is registered under the name, and the word is `command not
// found: zregexparse` on the line that wrote it — so a module naming one can
// still load and the script still finds out, loudly, at the point it depends
// on it. A missing *parameter* had no such call site: `${#jobstates}` on a
// shell without `$jobstates` is `0` at status 0, which is a plausible answer
// to a different question and reaches the caller as data. So the module could
// not load at all, and one absent parameter held every implemented one shut
// with it (#1058, #1146).
//
// A name registered here is the parameter's call site. It is not a stub and
// holds no value: nothing is stored, nothing is produced, and the name is not
// a parameter this shell has — [Runner.DynamicParameter] still says no, which
// is what keeps "refuses legibly" from being mistaken for "implemented". What
// it does is make the *read* say something:
//
//	zsh:3: jobstates: parameter not implemented yet
//
// and fail the expansion, so the command the value was for does not run with
// a value nobody produced. That is the same three things `command not found`
// does for a builtin — name it, refuse it, and let nothing depend on it.
//
// **Only where the script has no value of its own**, and not where it named
// its own answer for the absent case — see refuseAbsentParameter for both.
//
// The sentence is the dialect's, because which shell has the module and what
// it calls the shortfall are the dialect's. The *mechanism* — where the
// refusal fires and how hard it fails — is here, so a second dialect with
// modules cannot arrive at a second answer.

// SetAbsentParameter records that a name is a parameter this shell does not
// have, and says what a read of it is told. The reason is a sentence
// fragment: it follows the name and a colon.
func (r *Runner) SetAbsentParameter(name, reason string) {
	if r.absentParams == nil {
		r.absentParams = map[string]string{}
	}
	r.absentParams[name] = reason
}

// AbsentParameter reports whether a name is one this shell has not got but
// refuses by name when it is read.
//
// The question a module loader asks beside [Runner.DynamicParameter]: the
// two together are "everything this module provides is either implemented or
// refuses legibly", and neither one alone is that.
func (r *Runner) AbsentParameter(name string) bool {
	_, ok := r.absentParams[name]
	return ok
}

// refuseAbsentParameter is the read: it reports whether an expansion named an
// absent parameter, and refuses it if so.
//
// Asked at the top of both word paths — the scalar one and the list one — and
// not down where a value would have been fetched, because the routes that lose
// a read are the ones that never fetch a value at all. `${jobstates[x]}` is
// the one that matters: the array path answers a subscript of a name nothing
// holds with *no fields*, which never reaches the place `set -u` is asked, and
// `${functions[name]}` is the single most common thing a plugin manager writes.
// It read as empty at status 0 and said nothing, which is this bug wearing the
// exact shape the issue is about.
//
// Two things are deliberately not refused.
//
// **A name the script gave a value of its own.** `jobstates=(a b)` and then
// `$jobstates` is the script's array, exactly as it is in a shell where the
// module was never loaded. The refusal is about reading something absent, not
// about owning a spelling, and this test runs before any value is fetched —
// so it is the one place that can tell the two apart.
//
// **The four conditional operators.** `${jobstates-d}`, `${jobstates:=d}`,
// `${jobstates+x}` and `${jobstates?msg}` are a script saying what to do when
// the name has no value, and being told is what they are for: the first two
// supply one, the third answers "no", and the fourth reports in the script's
// own words. This is the same exemption `set -u` makes and for the same
// reason — an absent parameter answering *no* to "is it there?" is an answer,
// where one answering *empty* to "what is it?" is not.
func (r *Runner) refuseAbsentParameter(e *syntax.ParamExpr) bool {
	if e == nil {
		return false
	}
	reason, ok := r.absentParams[e.Name]
	if !ok {
		return false
	}
	switch e.Op {
	case syntax.ParamDefault, syntax.ParamAssign, syntax.ParamAlternate, syntax.ParamError:
		return false
	}
	if _, held := r.getVar(e.Name); held {
		return false
	}
	r.fatalExpansion("%s: %s\n", e.Name, reason)
	return true
}

// refuseAbsentParameterUnset is the *write* half, and it is `unset
// "parameters[PATH]"`: an element taken out of a table this shell has not got.
//
// The read half above is asked at the top of the word paths and never sees an
// `unset` operand, which is not an expansion and fetches nothing. So the
// commonest write on these tables walked straight past it, and what it walked
// into was the ordinary subscript machinery: the name holds nothing, so the
// brackets are read as arithmetic, and `unset "parameters[PATH]"` complained
// `invalid number: /opt/homebrew/bin:…` — a sentence about the *value of the
// key*, pointing at a math error nobody wrote. `unset "parameters[nope]"` was
// worse and was silent at status 0, which is this whole mechanism's own
// failure case wearing the shape it was built to stop (#1527).
//
// There is nothing to copy here. zsh's own answers for these names are
// incoherent between themselves — `read-only variable: nope` for
// `$parameters`, `job not found: x` for `$jobtexts`, silence for `$reswords`,
// and `$funcstack` and `$historywords` take the shell down with SIGSEGV — so
// the panel gives no behaviour to match and the rule this file already states
// is the answer: name it, refuse it, let nothing depend on it. The sentence is
// the read's, from the same table, so there is one of it.
//
// The script's own value is exempt for the read's reason: `parameters=(a b)`
// and then `unset "parameters[1]"` is the script's array, in this shell as in
// one where the module was never loaded.
func (r *Runner) refuseAbsentParameterUnset(name string) (string, bool) {
	reason, ok := r.absentParams[name]
	if !ok {
		return "", false
	}
	if _, held := r.getVar(name); held {
		return "", false
	}
	return reason, true
}

// An **absent element** is a key a produced association is asked for and
// whose producer has no answer. Reading it refuses by name; it never reads as
// empty.
//
// The same rule as an absent parameter, one level down, and it exists because
// a *partial* table is a shape the parameter half cannot describe. A name is
// either registered as produced or registered as absent, and a view that
// answers thirteen of the keys a script asks it for is both: the parameter is
// there, `${+name}` is 1, and a key it has no answer for is a hole in the
// middle of something that exists.
//
// An empty string is the wrong thing to put in that hole, and this is the one
// place where that is not a matter of taste. The thing being viewed can
// legitimately have nothing under a key — measured, real zsh's
// `$terminfo[colors]` is *absent* under `TERM=dumb`, because that terminal
// has no colors — so a caller reading empty cannot tell "the terminal lacks
// it" from "this shell never knew". #1388 is that confusion one layer up: a
// plugin manager's whole color table sits behind `-n ${terminfo[colors]}`,
// and the parameter being missing altogether read to it as a terminal with no
// colors, so every message it printed came out as raw markup. Answering the
// same shape from inside the parameter would have reproduced the bug with the
// parameter present, which is worse than the absence — the absence at least
// makes `${+terminfo}` say no.
//
// The three exemptions are the parameter half's, for the parameter half's
// reasons, and the third is new because a subscript has a spelling for the
// question that a bare name does not:
//
//   - **The four conditional operators.** `${terminfo[cnorm]-}` is a script
//     saying what to use when the key is not there, and it gets it.
//   - **A key the producer answers.** Refused only where there is no value,
//     so a table's own keys never reach this.
//   - **`${+terminfo[cnorm]}`.** The set test is the guard a well-written
//     script writes in front of the read — measured across a real plugin
//     tree, `$+terminfo[…]` is the *commonest* way these keys are touched —
//     and it is answered rather than refused because 0 is a true answer to
//     the question it asks. A guard that stops the shell is not a guard.

// SetAbsentElements records that a produced association answers only the keys
// its producer holds, and says what a read of any other key is told. The
// reason is a sentence fragment: it follows `name[key]` and a colon.
func (r *Runner) SetAbsentElements(name, reason string) {
	if r.absentElements == nil {
		r.absentElements = map[string]string{}
	}
	r.absentElements[name] = reason
}

// AbsentElements reports whether a name is a produced association that
// refuses the keys it has no answer for, and what it says.
func (r *Runner) AbsentElements(name string) (string, bool) {
	reason, ok := r.absentElements[name]
	return reason, ok
}

// refuseAbsentElement is the read: it reports whether an expansion asked a
// partial association for a key it does not answer, and refuses it if so.
//
// Asked from assocSubscript rather than from the two word paths, which is the
// opposite of where refuseAbsentParameter goes and for a reason that only
// applies here: the key has to be *expanded* to be known, and expanding a
// subscript twice runs whatever is in it twice. assocSubscript is the one
// place every element read converges on with the key already in hand, so
// there is nowhere for a route to slip past.
func (r *Runner) refuseAbsentElement(e *syntax.ParamExpr, key string) bool {
	if e == nil {
		return false
	}
	reason, ok := r.absentElements[e.Name]
	if !ok {
		return false
	}
	switch e.Op {
	case syntax.ParamDefault, syntax.ParamAssign, syntax.ParamAlternate, syntax.ParamError:
		return false
	}
	if e.SetTest {
		return false
	}
	r.fatalExpansion("%s[%s]: %s\n", e.Name, key, reason)
	return true
}
