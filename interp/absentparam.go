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
