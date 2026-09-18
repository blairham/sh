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
	if event == disciplineSet || event == disciplineAppend {
		// What the hook returned, for the assignment command to report. Kept
		// here rather than left in `r.status` because the store has more to
		// do afterwards and every step of it would overwrite the answer. See
		// Runner.disciplineStatus, and disciplineElementRead for the other half:
		// a `.get` puts the status back and a write does not.
		r.disciplineStatus, r.disciplineStatusSet = r.status, true
	}
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

// disciplineElementRead runs a variable's `.get` and reports the value the
// read should answer with, and whether the hook said anything at all.
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
//
// The element the read is about is carried in, and is what `${.sh.subscript}`
// answers inside the hook. A read of one element is a read and the same hook
// runs for it: measured on ksh93u+ 2012-08-01, 2026-09-16, `a=(p q); function
// a.get { … }; ${a[1]}` enters with `${.sh.name}` as `a` and
// `${.sh.subscript}` as `1`, and a hook that assigns `${.sh.value}` replaces
// that element alone. An unsubscripted read passes the empty subscript, which
// is what a scalar carries.
//
// Which subscript a read carries is measured rather than derived:
//
//   - `${a[@]}` and `${a[*]}` enter the hook once per element, each with its
//     own subscript, in the order the elements come out.
//   - A subscript the array has no element at fires all the same —
//     `a=(p q r); ${a[9]}` enters with `9` — where the same subscript on a
//     *scalar* fires nothing, because a scalar has one place and not nine.
//   - A negative subscript arrives counted forwards: `${a[-1]}` on three
//     elements enters with `2`.
//   - A bare `$a` on an indexed array is element `0` and says so; a bare read
//     of a scalar or of a keyed table carries the empty subscript.
//   - `${#a[@]}` and `${!a[@]}` fire nothing: neither is a read of a value.
func (r *Runner) disciplineElementRead(variable, subscript string) (string, bool) {
	if r.askingTheStoreOnly || !r.disciplineIsWatching(variable, disciplineGet) {
		return "", false
	}
	status, ctl := r.status, r.ctl
	v, assigned := r.runDiscipline(variable, disciplineGet, subscript, "", false)
	r.status, r.ctl = status, ctl
	return v, assigned
}

// disciplinedElement is one element's stored value after the name's `.get`
// has had the chance to replace it, and is the identity when there is no hook
// — which is every array in every shell but the one that has disciplines.
func (r *Runner) disciplinedElement(variable, subscript, stored string) string {
	if v, replaced := r.disciplineElementRead(variable, subscript); replaced {
		return v
	}
	return stored
}

// disciplinedElements is the same for a whole-array read, which enters the
// hook once per element with that element's own subscript.
//
// The subscripts come from the caller because only it knows them: an indexed
// array's are its keys, a keyed table's are the keys in the order its values
// came out, and the two are the same length as the values by construction.
// A read reaching here with no name behind it — the subscript on an
// expansion's own result — has no hook to find and is handed back untouched.
func (r *Runner) disciplinedElements(variable string, subscripts, values []string) []string {
	if !r.disciplineIsWatching(variable, disciplineGet) || len(subscripts) != len(values) {
		return values
	}
	out := make([]string, len(values))
	copy(out, values)
	for i := range out {
		out[i] = r.disciplinedElement(variable, subscripts[i], out[i])
	}
	return out
}

// exportedThroughDiscipline is the value an exported name reaches a child
// with, which is the one a *read* of it answers rather than the one the store
// holds.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-16: `g=raw; function g.get {
// .sh.value=A; }; export g; env` hands the child `g=A`, and so does a `ksh
// -c` started from the same shell. The environment is built from the tables,
// so without this the hook is the one reader nobody asks — and a discipline
// that computes a value would have been invisible to every command the script
// ran.
//
// No status to put back and none to take: disciplineElementRead already
// leaves `$?` where it found it, which is what keeps building an environment
// from being something a script can see in `$?`.
func (r *Runner) exportedThroughDiscipline(name, stored string) string {
	return r.disciplinedElement(name, "", stored)
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

// askTheStoreOnly marks the read that follows as one about **set-ness**, so
// the value's producer is not asked to produce a value nothing will look at.
//
// `${x+S}` is the shape: it answers the operand's word when the name is set
// and the empty string when it is not, and either way the parameter's value
// never reaches the result. Measured 2026-09-18 against ksh93u+ 2012-08-01
// from a script file under `env -i` with a scratch HOME, with `function x.get
// { print -u2 GET; }` and `x=V`:
//
//	${x}     one GET      ${x+S}    no GET at all
//	${x-D}   one GET      ${x+}     no GET at all
//	${x:-D}  one GET      [[ -v x ]]        no GET at all
//	${#x}    one GET      ${x:+S}   one GET
//	${x#V}   one GET      ${x=Z}    one GET
//
// The right column is the whole of the rule and the left is what keeps it
// narrow. A length needs a value, so `${#x}` runs the hook even though the
// value never reaches the result either — so this is not "the value is not
// returned". And the **colon** form runs it, because "unset or empty" is a
// question about the value and only the colon-less test is a question about
// the store. That pair is why it is a mark on the read rather than a list of
// operators judged by what they return.
//
// The note in Runner.varValue is the other half of the same rule and was
// already right — *"set-ness stays the store's either way"* — except that the
// hook ran in front of it (#3121).
//
// Not an axis: a discipline is one shell's alone, so there is one column and
// it is the one being matched. Every other dialect has no hook to skip.
func (r *Runner) askTheStoreOnly() func() {
	was := r.askingTheStoreOnly
	r.askingTheStoreOnly = true
	return func() { r.askingTheStoreOnly = was }
}

// unsetDiscardsTheDisciplines takes a variable's hooks away with the variable.
//
// **The binding is to the variable and not to the name**, so removing the
// variable removes the functions that were watching it. Measured 2026-09-18
// against ksh93u+ 2012-08-01 from a script file under `env -i` with a scratch
// HOME, with `s=1` and `function s.set { … }` standing:
//
//	unset s; typeset +f                  lists nothing — an ordinary
//	                                     function beside it survives
//	unset s; s.set                       `s.set: not found`
//	unset s; whence -v s.set             `not found`
//	unset s; s=2                         no hook
//	unset -v s                           the same, by the other spelling
//	typeset -A m=(…); unset m            the same for a table
//	unset s; function s.set { … }; s=3   fires again — the definition rebinds
//
// A name **nothing has set** is the row that says this is the variable going
// and not the `unset` word: `function s.set { … }; unset s; s=1` still fires,
// because there was no variable for the removal to be about. It is the same
// guard the `.unset` hook itself is behind.
//
// #3162 filed this the other way round — that `unset` merely detaches a
// function which stays listed and callable — on the strength of a `typeset -f
// | grep -c` that answered 1. The three direct probes above answer the
// question that count was standing in for, and all three say the function is
// gone.
//
// Not an axis: a discipline is one shell's alone, so the shell that has them
// is the one being matched and every other dialect reaches this with an empty
// table.
func (r *Runner) unsetDiscardsTheDisciplines(variable string) {
	if !r.disciplined[variable] {
		return
	}
	for event := range disciplineEvents {
		r.removeFunctionQuietly(variable + "." + event)
	}
	delete(r.disciplined, variable)
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
