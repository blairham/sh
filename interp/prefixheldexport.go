// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What a command's route does to the **export attribute** of a prefix entry,
// asked in two places that must not disagree: where the route stores the entry
// for real, and where a walk holds it ahead of that store for the entries and
// operands behind it — see interp/prefixsees.go.
//
// The hold used to write the value and leave the attribute as the name had it,
// so a command substitution behind the entry saw whatever the shell's own name
// was. Measured 2026-10-30 from `-c` under `env -i PATH=/usr/bin:/bin`, with
// `printenv` being /usr/bin/printenv and `f() { echo "w=$w"; }`:
//
//	                                                     ksh93u+  zsh 5.9.2  bash 5.3  dash
//	v=old; v=new w=$(printenv v) printenv w              new      new        new       new
//	v=old; v=new w=$(printenv v) f                       w=       w=new      w=new     w=new
//	v=old; v=new w=$(printenv v) eval 'echo w=$w'        w=       w=         w=new     w=
//	export v=old; v=new w=$(printenv v) eval 'echo …'    w=       w=new      w=new     w=new
//	export v=old; v=new typeset y=$(printenv v)          y=       y=new
//	v=old; v=new export y=$(printenv v)                  y=new    y=
//
// So the entry behind sees the attribute the route gives the entry in front of
// it, in every column: a program's prefix is its environment and is exported
// everywhere; a function's and a builtin's follow the axes that already say
// what the route's store does — PrefixToAFunctionIsExported,
// PrefixToAKeywordFunctionIsScopedToTheCall and PrefixExportAtABuiltin, whose
// fourth answer is ksh93's `export` exporting its own prefix. This shell held every one of them with the name's
// own attribute: the first row was empty in all five dialects, bash's
// function and `eval` rows were empty, and ksh93's builtin and POSIX-form
// function rows read the old value through a name that was exported before
// the command (#7003).

// heldExportRoute is the command a walk holds its prefix in front of, as far
// as the export attribute goes. The zero value is a walk that does not know,
// which leaves the attribute as the name has it.
type heldExportRoute struct {
	set    bool
	kind   prefixCommand
	scoped bool
}

// heldExportRouteOf reads the command a prefix stands in front of for a walk
// that holds the prefix's entries. Nothing here asks: an axis this reads
// unanswered moves nothing, and the route asks it where it stores.
func (r *Runner) heldExportRouteOf(argv []string) heldExportRoute {
	if len(argv) == 0 {
		return heldExportRoute{}
	}
	route := heldExportRoute{set: true, kind: r.prefixCommandOf(argv)}
	if route.kind.kind == prefixBeforeFunction {
		if fn := r.funcs[route.kind.word]; fn != nil && fn.Keyword {
			// A traced call's prefix is held for the front pass first, which
			// is the shell's own: `function g { :; }; typeset -ft g; x=1
			// y=$(printenv x) g` leaves `y` empty in ksh93u+. The pass behind
			// it is held as the call's. See interp/tracedcallprefix.go.
			route.scoped = r.sem().PrefixToAKeywordFunctionIsScopedToTheCall == Yes &&
				!r.tracedCallPersistsItsPrefixFirst(route.kind.word)
		}
	}
	return route
}

// heldPrefixExport is what the route gives a scalar entry over name, for a
// walk holding it: on is the attribute and moves whether there is one to set.
func (r *Runner) heldPrefixExport(route heldExportRoute, name string) (on, moves bool) {
	switch route.kind.kind {
	case prefixBeforeExternal:
		// The child's environment, which is unanimous.
		return true, true
	case prefixBeforeFunction:
		return r.prefixExportAtAFunction(route.scoped, false)
	}
	return r.prefixExportAtThisBuiltin(route.kind, name, false)
}

// prefixExportAtThisBuiltin is what a builtin's route does to the export
// attribute of a scalar prefix entry over name, for the length of the builtin.
//
// asking is true for the route, which refuses an unanswered axis by name. A
// walk holding the entry ahead of it passes false and reads the answer, so
// the refusal is never written twice.
func (r *Runner) prefixExportAtThisBuiltin(kind prefixCommand, name string, asking bool) (on, moves bool) {
	policy := r.sem().PrefixExportAtABuiltin
	off := policy == PrefixExportAtABuiltinOff || policy == PrefixExportAtABuiltinOffExceptAtExport
	if kind.throughCommand && (kind.kind == prefixBeforeExternal || !off) {
		// `command` is a precommand word rather than a command, so the prefix
		// in front of it belongs to whatever it goes on to run — and when that
		// is an external, belonging to it means being in its environment.
		// Setting the name is not enough for that: a child is handed the
		// *exported* names, so `v=1; v=9 command env` showed the child nothing
		// where bash 5.3, ksh93, zsh and dash all show it `v=9` (#2408).
		// Unanimous, so it is done here and not asked.
		//
		// Only through `command`. A prefix before a builtin that runs text of
		// its own is where the panel splits — bash shows the child `v=9` for
		// `v=9 eval env` and the other three show it nothing — so exporting
		// for every builtin would take three columns with it to answer one.
		return true, true
	}
	if policy == PrefixExportAtABuiltinOffExceptAtExport && kind.word == "export" {
		// The one builtin that exports its own prefix in the column that
		// takes the attribute off everywhere else. See
		// PrefixExportAtABuiltinOffExceptAtExport.
		return true, true
	}
	// Every other builtin, where the panel does split and the attribute moves
	// in both directions: bash hands the builtin an environment and exports
	// the name, ksh93 reads the prefix as an ordinary assignment and takes the
	// attribute off a name that had it, and the rest leave it exactly as it
	// was. See Semantics.PrefixExportAtABuiltin.
	//
	// `command` in front of a builtin is that builtin's prefix in the column
	// that takes the attribute off: measured 2026-10-16 on ksh93u+, `v=9
	// command eval '/usr/bin/printenv v'` finds nothing and `$v` is 9
	// afterwards, where bash and dash hand the child `v=9` (#6859).
	switch {
	case asking:
		on, moves = r.prefixExportAtABuiltin()
	case policy == PrefixExportAtABuiltinOn:
		on, moves = true, true
	case off:
		on, moves = false, true
	}
	if moves && !on && r.caseLetterKeepsTheExport(name) {
		// And a name carrying a case letter keeps it there: `typeset -xu
		// v=x; v=abc :` hands a child `v=ABC`, where `typeset -x v=x; v=abc
		// :` hands it nothing.
		return false, false
	}
	return on, moves
}

// prefixExportAtAFunction is what a function's route does to the export
// attribute of a scalar prefix entry for the length of the call. asking is
// prefixExportAtThisBuiltin's.
//
// A cell the call owns is exported for the call, whatever the axis says about
// a prefix the shell owns: the prefix is the call's environment, and every
// child the body starts is handed it. See
// Semantics.PrefixToAKeywordFunctionIsScopedToTheCall.
//
// Otherwise the two readings move the attribute in opposite directions rather
// than one of them leaving it alone: where the prefix is the command's
// *environment* the name gains the attribute, and where it is a plain
// assignment to this shell the name **loses** it. So the axis is asked on
// every name, including one that was exported before the call — `export v=1;
// f(){ env; }; v=9 f` hands the child `v=9` in six columns and tells it
// nothing in ksh93, where `typeset -p v` inside the body prints a plain `v=9`.
// An earlier reading asked only on a name that was not exported already, on
// the premise that one that was reaches every child either way; the ksh93
// column is what that premise is false in.
func (r *Runner) prefixExportAtAFunction(scoped, asking bool) (on, moves bool) {
	if scoped {
		return true, true
	}
	answer := r.sem().PrefixToAFunctionIsExported
	if !asking {
		return answer == Yes, answer == Yes || answer == No
	}
	on = r.ask(answer, "an assignment before a function being exported for the call")
	return on, on || answer == No
}
