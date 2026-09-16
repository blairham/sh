// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A *discipline* is a function the shell runs because something happened to a
// variable, rather than because a command named it.
//
// One shell in the panel has them. `function g.get { … }` there is not a
// function called `g.get`; it is the hook run whenever `g` is read, and
// `.set`, `.append` and `.unset` are the same for an assignment, an append
// and an `unset`. The event carries its value in `${.sh.value}`, the name it
// fired for in `${.sh.name}`, and the element in `${.sh.subscript}` — three
// parameters that exist only while a hook is running, which is why they are
// saved and put back rather than stored.
//
// The core keeps the mechanism and names none of it. The spelling of the
// suffixes is not a dialect's to vary — a hook is a hook or the shell has
// none — so the four words are here; what a dialect answers is whether a
// dotted name means this at all. See
// [Semantics.DisciplineFunctionIsAVariableHook], and dialect/ksh for the
// column that says yes.
//
// Measured against AT&T ksh93u+ 2012-08-01 on 2026-09-15, and the two answers
// worth naming before they are read are the ones that look like mistakes:
//
//   - `.get` is entered with `${.sh.value}` **empty**, not with the value the
//     read is about. A hook that leaves it alone lets the real value through;
//     one that assigns it — even to the empty string — replaces the read.
//     `g=raw; function g.get { .sh.value=""; }; echo "[$g]"` is `[]` there,
//     where the same hook with an empty body is `[raw]`. So what decides is
//     whether the hook *assigned*, not what the parameter ends up holding,
//     and comparing values cannot tell the two apart.
//   - `.append` is given only the part being appended. `p=base; p+=more`
//     enters `p.append` with `${.sh.value}` as `more`, and rewriting it to
//     `<more>` leaves `p` as `base<more>`.
//
// And two that keep the hooks from eating themselves:
//
//   - A hook does not re-enter itself. A `.get` that reads its own variable
//     reads the stored value; a `.set` that assigns its own variable is not
//     re-entered by that assignment. The guard is per *event*, not per
//     variable — measured, a `.get` that assigns its variable does fire that
//     variable's `.set`.
//   - `.get` leaves `$?` alone and `.set` does not: `function g.get { return
//     5; }` reads at status 0, where `function s.set { return 5; }; s=1` is
//     status 5.
const (
	disciplineGet    = "get"
	disciplineSet    = "set"
	disciplineAppend = "append"
	disciplineUnset  = "unset"
)

// The parameters a hook is entered with. Ordinary dotted variable names, so
// a script reads them with `${.sh.value}` and assigns them back the same way
// — there is nothing special about them except when they exist.
const (
	disciplineValueParam     = ".sh.value"
	disciplineNameParam      = ".sh.name"
	disciplineSubscriptParam = ".sh.subscript"
)

// disciplineEvents is the set of suffixes that name an event. A dotted name
// whose suffix is not one of these is not a discipline and is refused — which
// is the row `function ns.thing` is on, and the one this shell already had
// right.
var disciplineEvents = map[string]bool{
	disciplineGet:    true,
	disciplineSet:    true,
	disciplineAppend: true,
	disciplineUnset:  true,
}

// disciplineTarget splits a function name into the variable it watches and
// the event it watches for.
//
// Exactly one dot, a plain name in front of it and a known event behind it.
// Two dots is a different complaint in the shell being modeled — `function
// a.b.get` is `a.b: no parent` there, a compound variable's question rather
// than a function's — and is left to the refusal the caller already has.
func disciplineTarget(fname string) (variable, event string, ok bool) {
	dot := strings.IndexByte(fname, '.')
	if dot <= 0 || dot == len(fname)-1 {
		return "", "", false
	}
	variable, event = fname[:dot], fname[dot+1:]
	if !disciplineEvents[event] || !isPlainFuncName(variable) {
		return "", "", false
	}
	return variable, event, true
}

// definesADiscipline reports whether this dialect reads the name as a hook,
// and records the variable so that a read need not build a string to find
// out that there is nothing to run.
//
// The function is stored under its whole name like any other, which is not
// only the cheap implementation: the shell being modeled lists it with
// `typeset -f g.get`, calls it when a command names it, and forgets the hook
// when `unset -f g.get` takes the function away. One table serves all four.
func (r *Runner) definesADiscipline(fname string) bool {
	variable, _, ok := disciplineTarget(fname)
	if !ok {
		return false
	}
	if !r.ask(r.sem().DisciplineFunctionIsAVariableHook,
		"a dotted function name being a variable's discipline") {
		return false
	}
	if r.disciplined == nil {
		r.disciplined = map[string]bool{}
	}
	r.disciplined[variable] = true
	return true
}

// disciplineHook is the function to run for one event on one variable, or
// nothing at all — which is the answer almost every time it is asked, so the
// first line is a nil-map check and not a string being built.
func (r *Runner) disciplineHook(variable, event string) (*syntax.FuncDecl, string, bool) {
	if !r.disciplined[variable] || r.ctx == nil {
		return nil, "", false
	}
	name := variable + "." + event
	fn, ok := r.funcs[name]
	if !ok || r.disciplineRunning[name] {
		// Not defined, or already running one frame down — see the note on
		// re-entry above, which is what keeps a `.get` that reads its own
		// variable from being a loop instead of an answer.
		return nil, "", false
	}
	return fn, name, true
}

// disciplineIsWatching reports whether an event on this variable has a hook
// waiting, without running it. For the store that has to know before it has
// a value to hand over.
func (r *Runner) disciplineIsWatching(variable, event string) bool {
	_, _, ok := r.disciplineHook(variable, event)
	return ok
}

// runDiscipline runs one hook and reports what `${.sh.value}` held when it
// returned, and whether the hook left it there at all.
//
// valueGiven says whether the event carries a value into the hook. `.set` and
// `.append` do — the value being assigned, and the part being appended — and
// `.get` and `.unset` do not, which is what makes "the hook assigned it" a
// question with an answer for a read.
func (r *Runner) runDiscipline(variable, event, subscript, value string, valueGiven bool) (string, bool) {
	fn, name, ok := r.disciplineHook(variable, event)
	if !ok {
		return "", false
	}
	if r.disciplineRunning == nil {
		r.disciplineRunning = map[string]bool{}
	}
	r.disciplineRunning[name] = true
	defer delete(r.disciplineRunning, name)

	restore := r.enterDisciplineParams(variable, subscript, value, valueGiven)
	// A hook is entered with no positional parameters of its own — measured,
	// `$#` inside one is 0 however the shell was called — which callFunc
	// gives it by being handed none.
	err := callDisciplineBody(r, fn)
	got, assigned := r.Vars[disciplineValueParam]
	restore()
	if err != nil {
		return "", false
	}
	return got, assigned
}

// enterDisciplineParams gives the three parameters their values for the hook
// and hands back the way to put the shell's own back.
//
// Saved and restored rather than set and cleared, because a hook may run
// inside another one: a `.get` whose body reads a second variable with a
// `.get` of its own comes back to find its own `${.sh.value}` still there in
// the shell being modeled.
func (r *Runner) enterDisciplineParams(variable, subscript, value string, valueGiven bool) func() {
	type saved struct {
		value string
		set   bool
	}
	names := [...]string{disciplineNameParam, disciplineSubscriptParam, disciplineValueParam}
	var was [len(names)]saved
	for i, n := range names {
		v, ok := r.Vars[n]
		was[i] = saved{v, ok}
	}
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	r.Vars[disciplineNameParam] = variable
	r.Vars[disciplineSubscriptParam] = subscript
	if valueGiven {
		r.Vars[disciplineValueParam] = value
	} else {
		// Absent rather than empty, which is the whole of how a `.get` that
		// assigned is told from one that did not.
		delete(r.Vars, disciplineValueParam)
	}
	return func() {
		for i, n := range names {
			if was[i].set {
				r.Vars[n] = was[i].value
				continue
			}
			delete(r.Vars, n)
		}
	}
}

// disciplineRead runs a variable's `.get` and reports the value the read
// should answer with, and whether the hook said anything at all.
//
// It answers only for a hook that *assigned* `${.sh.value}`; a hook that did
// not leaves the read to the store, and the store is read afterwards rather
// than before. That order is measured: a `.get` whose body assigns its own
// variable — `function g.get { g=written; }` — makes the very read that ran
// it answer `written`, so a value fetched first would be one line stale.
//
// `$?` is put back afterwards, which `.set` does not get: measured,
// `function g.get { return 5; }` reads at status 0, where `function s.set {
// return 5; }; s=1` is status 5.
func (r *Runner) disciplineRead(variable string) (string, bool) {
	if !r.disciplineIsWatching(variable, disciplineGet) {
		return "", false
	}
	status, ctl := r.status, r.ctl
	v, assigned := r.runDiscipline(variable, disciplineGet, "", "", false)
	r.status, r.ctl = status, ctl
	return v, assigned
}

// dottedFunctionNameIsWellFormed reports whether a name `unset -f` is given
// is one a function could have had, in the dialect where a dot in a function
// name means something.
//
// Measured on ksh93u+ 2012-08-01: `unset -f g.get` and `unset -f ns.thing`
// are both silent at 0 — the second one is a name no definition would have
// accepted — where `unset -f 1x` and `unset -f f-g` are
// `invalid function name` at 1. So the builtin judges the *shape* and leaves
// which suffixes are events to the definition, which is one rule and not a
// list (#3033).
func (r *Runner) dottedFunctionNameIsWellFormed(name string) bool {
	if !strings.ContainsRune(name, '.') {
		return false
	}
	for _, part := range strings.Split(name, ".") {
		if !isPlainFuncName(part) {
			return false
		}
	}
	return r.ask(r.sem().DisciplineFunctionIsAVariableHook,
		"a dotted function name being a variable's discipline")
}

// disciplineWrite runs a `.set` or `.append` and reports the value to store.
//
// The value always goes in and always comes back out, so a hook that does not
// touch `${.sh.value}` is the identity — which is what makes a hook that only
// prints leave the assignment alone.
func (r *Runner) disciplineWrite(variable, event, subscript, value string) (string, bool) {
	if !r.disciplineIsWatching(variable, event) {
		return "", false
	}
	v, assigned := r.runDiscipline(variable, event, subscript, value, true)
	if !assigned {
		// The hook unset the parameter, which no measured script does; the
		// value it was handed is what the store gets.
		return value, true
	}
	return v, true
}

// suppressDiscipline marks an event as already being handled, so that a store
// one layer down does not fire it a second time, and hands back the way to
// lift the mark.
//
// Two callers, both measured. An append fires `.append` and **not** `.set` —
// `p=base; function p.set { … }; p+=more` runs nothing at all there — and the
// store an append ends at is the same scalar store a plain assignment uses.
// An element write fires `.set` once with its subscript, and the store it
// ends at keeps the whole name's scalar view in step through that same store.
func (r *Runner) suppressDiscipline(variable, event string) func() {
	if !r.disciplined[variable] {
		return func() {}
	}
	name := variable + "." + event
	if r.disciplineRunning[name] {
		return func() {}
	}
	if r.disciplineRunning == nil {
		r.disciplineRunning = map[string]bool{}
	}
	r.disciplineRunning[name] = true
	return func() { delete(r.disciplineRunning, name) }
}

// disciplineUnsetName runs a variable's `.unset` hook before the name goes.
//
// Before, and that is measured rather than convenient: the hook reads the
// variable's own value and gets the one it is about to lose. Nothing comes
// back — the removal happens either way, and a hook that unsets the name
// itself is not a loop because the event does not re-enter.
func (r *Runner) disciplineUnsetName(variable string) {
	if !r.disciplineIsWatching(variable, disciplineUnset) {
		return
	}
	r.runDiscipline(variable, disciplineUnset, "", "", false)
}

// callDisciplineBody runs a hook's body, through a variable so that the
// package still compiles.
//
// A hook is reached from a variable *read*, and a read is reached from the
// builtin table's own initialization — the escape decoder asks the locale,
// which asks a variable. Naming callFunc here directly makes that a cycle Go
// refuses to order, the same one interp/runner.go's varValue already
// documents for arithmetic. Assigned in init, which runs after the tables
// are built, so the reference is no longer one the initializer graph sees.
var callDisciplineBody func(r *Runner, fn *syntax.FuncDecl) error

func init() {
	callDisciplineBody = func(r *Runner, fn *syntax.FuncDecl) error {
		return r.callFunc(r.ctx, fn, nil)
	}
}
