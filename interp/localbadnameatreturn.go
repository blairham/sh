// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// badNamesHeldForTheReturn takes out of a `local` line the operands whose
// name is not a name but which carry a value, in the dialect that says
// nothing about them until the call returns, and keeps them on the running
// call's scope. See Semantics.LocalBadNameWithAValueIsRefusedAtTheReturn.
//
// Asked only where such an operand is on the line, so an ordinary `local`
// puts no question to anybody.
func (r *Runner) badNamesHeldForTheReturn(word string, args []string, nowhere bool) []string {
	if nowhere || len(r.scopes) == 0 {
		return args
	}
	_, takes := r.nameRules(word)
	var kept []string
	for i, a := range args {
		name, _, hasValue := strings.Cut(a, "=")
		if !hasValue || r.isBuiltinName(word, name, takes) {
			if kept != nil {
				kept = append(kept, a)
			}
			continue
		}
		if !r.ask(r.sem().LocalBadNameWithAValueIsRefusedAtTheReturn,
			"a `local` operand with a bad name and a value refused when the call returns") {
			return args
		}
		if kept == nil {
			kept = append([]string{}, args[:i]...)
		}
		sc := r.scopes[len(r.scopes)-1]
		sc.badNamesForTheReturn = append(sc.badNamesForTheReturn, name)
	}
	if kept == nil {
		return args
	}
	return kept
}

// refuseTheBadNamesHeldForTheReturn is the refusal badNamesHeldForTheReturn
// put off: the first name, in the words a bad name gets, located where the
// shell has got to rather than at the `local`, and fatal.
//
// Measured 2026-10-03 in the pinned image over a script file: a body of
// `local 1x=5` / `echo "in=$?"` / `echo two` called from line 7 writes
// `in=0` and `two` and then `<script>: line 4: 1x: bad variable name` — the
// body's last line, with no `local:` in front of it — and the shell ends at
// 2. One name is reported where two were held, as the first refusal ends
// the shell.
func (r *Runner) refuseTheBadNamesHeldForTheReturn(sc *scope) {
	if len(sc.badNamesForTheReturn) == 0 || r.ctl == controlExit {
		return
	}
	name := sc.badNamesForTheReturn[0]
	outer := r.inBuiltin
	r.inBuiltin = ""
	status := r.badBuiltinName("local", name, name, Yes)
	r.inBuiltin = outer
	r.status = status
}
