// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// dottedNameHasNoParent reports whether a dotted name's parent is missing: the
// name before its last dot is nothing the shell holds, and no compound
// variable stands above it. A name with no dot past its first byte has no
// parent to miss.
func (r *Runner) dottedNameHasNoParent(name string) bool {
	if i := strings.IndexByte(name, '['); i > 0 {
		name = name[:i]
	}
	if !r.dottedName(name) {
		return false
	}
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return false
	}
	parent := name[:i]
	// A path that starts at a **reference** is a path from what it points at,
	// and the reference itself is a parent that is always there: measured
	// 2026-10-15, `typeset -n e=c; e.x=9` is stored with `c` unset, and `c=1;
	// typeset -n e=c; e.x.y=9` is `e.x.y=9: no parent`. One that points at
	// nothing is the reference machinery's refusal (`u: no reference name`),
	// not this one.
	if dot := strings.IndexByte(name, '.'); dot > 0 && r.isNameref(name[:dot]) {
		target, aimed := r.namerefTarget(name[:dot])
		if !aimed || i == dot {
			return false
		}
		parent = target + parent[dot:]
	}
	// `.sh` is the shell's own namespace and is always there, though nothing
	// is stored under the name itself: `{.sh.x}` is taken where `{.sh.q.r}`
	// is `no parent`, measured. And so is a namespace a `namespace NAME { …
	// }` block declared: `.ns.y=2` is stored.
	if parent == ".sh" || (parent[0] == '.' && r.namespaces[parent[1:]]) || r.namerefMemberParentIsThere(parent) {
		return false
	}
	for j := 1; j < len(parent); j++ {
		if parent[j] == '.' && r.isCompoundVariable(parent[:j]) {
			return false
		}
	}
	return true
}

// refuseAStoreWithNoParent refuses an assignment to a dotted name whose
// parent is missing, and reports whether it did. Measured 2026-10-15 on
// ksh93u+ 2012-08-01 from a script file (#6628):
//
//	a.b=1                    t.sh: line 1: a.b=1: no parent, ends at 1
//	a.b.c=1, a=1; a.b.c=1    a.b.c=1: no parent — the parent is a.b
//	a.b+=x, a.b[1]=x         a.b+=x: no parent, a.b[1]=x: no parent
//	a.b=(x y)                a.b: no parent
//	a.b=$(echo v)            a.b=v: no parent — the value runs first
//	typeset a.b=1            a.b=1: no parent, no builtin named
//	f(){ a.b=1; }; f || :    ends the script all the same
//	(a.b=1); echo $?         1 — the subshell is what ends
//	a=1; a.b=2               stored: a parent holding a scalar is there
//	typeset -C a; a.b.c=1    stored: a compound stands above it
//	.sh.x=1                  stored: `.sh` is always there
//
// So the sentence names the assignment as written, its value expanded, and
// the refusal is fatal wherever it is made: no `||` catches it and no
// function returns from it.
func (r *Runner) refuseAStoreWithNoParent(a *syntax.Assign) bool {
	w := r.diag().NoParent
	if w == "" || !r.dottedNameHasNoParent(a.Name) {
		return false
	}
	shown := a.Name
	if !a.IsArray && a.Members == nil {
		value := r.assignValue(a)
		if r.assignmentGaveUp() {
			// The value did not expand, and that has said so: the store is
			// never reached.
			return true
		}
		shown = r.assignmentAsWritten(a, value)
	}
	r.diagf("%s\n", Wording(w, "%[1]s: no parent", shown))
	r.status = 1
	r.fatalQuiet()
	return true
}

// refuseAnOperandWithNoParent is that refusal for a declaration's operand,
// and for `unset`'s. Measured 2026-10-15 on ksh93u+ 2012-08-01 from a script
// file (#6628):
//
//	typeset a.b=1, integer a.b=1     a.b=1: no parent — no builtin named
//	typeset -x a.b=1                 the same, ahead of the bad-name refusal
//	typeset -x a.b, typeset -n a.b=c is not an identifier — the letter's
//	typeset a.b, typeset -i a.b      t.sh[1]: typeset: a.b: no parent
//	integer a.b, float a.b           typeset: — the builtin they are
//	readonly a.b                     t.sh[1]: readonly: a.b: no parent
//	unset a.b                        t.sh[1]: unset: a.b: no parent, st=1
//
// Every declaration ends the script there, `||` or a function around it or
// not; `unset` carries on at 1. export is not here: a dotted operand to it is
// `is not an identifier` with or without a parent — see
// Diagnostics.BadNameOfADottedOperandWithAValue.
func (r *Runner) refuseAnOperandWithNoParent(builtin, operand, name string) (refused, ended bool) {
	w := r.diag().NoParent
	if w == "" {
		return false, false
	}
	speaker := builtin
	switch builtin {
	case "typeset", "readonly", "unset", "read", "let":
	case "integer", "float":
		speaker = "typeset"
	default:
		return false, false
	}
	if r.dottedOperandRefusedByALetter && (operand == name || r.dottedOperandRefusedWithAValue) {
		// Not a name to the letter at all, which is said first: `typeset -x
		// nosuch.x` and `typeset -n a.b=c` are `is not an identifier`, where
		// `typeset -x a.b=1` is this.
		return false, false
	}
	if !r.dottedNameHasNoParent(name) {
		return false, false
	}
	if operand != name {
		// With a value the sentence is the assignment's, located as the
		// script's own line rather than as the builtin's.
		outer := r.inBuiltin
		r.inBuiltin = ""
		r.diagf("%s\n", Wording(w, "%[1]s: no parent", operand))
		r.inBuiltin = outer
	} else {
		r.diagf("%s: %s\n", speaker, Wording(w, "%[1]s: no parent", name))
	}
	r.status = 1
	if builtin == "unset" || builtin == "read" || builtin == "let" {
		return true, false
	}
	r.fatalUsageQuiet()
	return true, true
}

// isTheShellsOwnNamespace reports whether the name is `.sh` itself, in the
// grammar that has dotted names: the namespace every `.sh.` member lives in,
// which is always there and which nothing may be stored into. Measured
// 2026-10-15 on ksh93u+ 2012-08-01 from a script file (#6628):
//
//	.sh=1, .sh+=x, .sh=(a b)      t.sh: line 1: .sh: is read only, ends at 1
//	typeset .sh=1                 the same
//	f() { .sh=1; echo in; }; f    the same; `in` is never written
//	true {.sh}</dev/null          .sh: is read only, st=1 — see
//	                              Runner.fdVarNameFailsFirst
//	.sh.x=1, .shx=1               stored: neither is the namespace
//
//	unset .sh, read .sh           unset: warning: .sh: is read only, st=1
//
// So it is refused as a readonly name is, wording and fatality alike, without
// carrying the letter: `readonly -p` does not list it there, and neither does
// `typeset -p`.
func (r *Runner) isTheShellsOwnNamespace(name string) bool {
	return name == ".sh" && r.lang().DottedName
}

// nameIsFrozen reports whether a store into the name is refused as a
// readonly one is: the name carries the letter, or it is the shell's own
// namespace. Asked by the three refusals that write the sentence — the
// assignment's, `unset`'s and `read`'s — rather than by every reader of the
// letter, most of which are asking about the letter itself.
func (r *Runner) nameIsFrozen(name string) bool {
	return r.attrHas(name, attrReadonly) || r.isTheShellsOwnNamespace(name)
}

// assignmentAsWritten is an assignment the way ksh93 quotes one back in a
// refusal: the name, the subscript as written, the operator, and the value
// expanded.
func (r *Runner) assignmentAsWritten(a *syntax.Assign, value string) string {
	shown := a.Name
	if a.Index != nil {
		shown += "[" + r.subscriptAsWritten(a.Index) + "]"
	}
	if a.Append {
		return shown + "+=" + value
	}
	return shown + "=" + value
}

// refuseADottedPrefix refuses an assignment prefix to a dotted name where the
// command does not take it, and reports whether it did. Two refusals, and the
// command kind picks between them. Measured 2026-10-15 on ksh93u+ 2012-08-01
// from a script file (#6628):
//
//	a.b=1 :, a.b=1 eval …, a.b=1 pf        a.b=1: no parent, the script ends
//	typeset -C a; a.b=1 :                  stored
//	a.b=1 true, a.b=1 /usr/bin/true        a.b=1: is not an identifier, st=1
//	typeset -C a; a.b=1 /usr/bin/true      the same — a parent does not help
//	.sh.x=1 true, .sh=1 /usr/bin/true      the same
//	function g {…}; a.b=1 g                the same sentence, and it ends
//	x=$(echo S >&2) a.b=1 /usr/bin/true    S, then the refusal
//	a.b=1 x=$(echo S >&2) /usr/bin/true    the refusal, and no S
//
// So a prefix that **stores** — a special builtin's, a POSIX-form
// function's — is an assignment and meets the missing parent the way one
// does, while a prefix the command is only *shown* — a regular builtin's, a
// program's, a `function`-form call's — will not take a dotted name at all.
// That one is a failure of the prefix, which the walk reads as an expansion
// failure: the entries behind it are not expanded, and the command kind
// decides whether it costs the command or the script, exactly as `a=$((1/0))`
// in the same place does. See interp/prefixexpansionfailed.go.
//
// value is the entry's expansion, and unjoined says an append's join is still
// to be made — made here, behind the dotted test, so that no other entry pays
// for it.
func (r *Runner) refuseADottedPrefix(a *syntax.Assign, value string, unjoined, stores bool) bool {
	if a.Operand || !r.dottedName(a.Name) {
		return false
	}
	d := r.diag()
	wording := d.NoParent
	if !stores {
		wording = d.BadNameOfADottedOperandWithAValue
	}
	if wording == "" || (stores && !r.dottedNameHasNoParent(a.Name)) {
		return false
	}
	if unjoined {
		value = r.prefixJoined(a, value)
	}
	shown := r.assignmentAsWritten(a, value)
	if stores {
		r.diagf("%s\n", Wording(wording, "%[1]s: no parent", shown))
		r.status = 1
		r.fatalQuiet()
		return true
	}
	// Located as a value that would not expand is, which is the builtin's
	// location in front of a regular builtin.
	defer r.speakForAPrefixValue()()
	r.diagf("%s\n", Wording(wording, "%[2]s: is not an identifier", "", shown))
	r.status, r.expandErr = 1, true
	return true
}

// dottedPrefixIsNotTaken reports, without saying anything, whether
// refuseADottedPrefix will refuse this entry as a name the command is only
// shown: what the trace written after the command stops at, since ksh93
// writes no line for the entry it refuses — measured, `set -x; a.b=1 true`
// is `+ true` and the refusal, with no `+ a.b=1` between them.
func (r *Runner) dottedPrefixIsNotTaken(a *syntax.Assign, p prefixCommand) bool {
	if p.kind != prefixBeforeExternal && p.kind != prefixBeforeRegularBuiltin {
		return false
	}
	return !a.Operand && r.diag().BadNameOfADottedOperandWithAValue != "" && r.dottedName(a.Name)
}

// refuseALoopNameWithNoParent refuses a `for` or `select` variable that is a
// dotted name whose parent is missing, and reports whether it did. Measured
// 2026-10-16 on ksh93u+ 2012-08-01 from a script file:
//
//	for a.b in 1; do :; done; echo st   t.sh: line 1: a.b: no parent, ends
//	for a.b in $(echo X >&2); do …      X, then the refusal: the words first
//	for a.b; do :; done                 refused with nothing to iterate too
//	for a.b in 1; do :; done > f        f is made: the clause's redirection
//	select a.b in 1; do break; done     the refusal, and no menu
//
// So it is the store's refusal, fatal wherever it is made, and asked once the
// words are expanded rather than per pass (#6794).
func (r *Runner) refuseALoopNameWithNoParent(name string) bool {
	return r.refuseAStoreToANameWithNoParent(name)
}

// refuseAStoreToANameWithNoParent is the refusal a store to a name with no
// parent makes where no assignment was written — a loop's variable and an
// arithmetic expression's — fatal as the assignment's is, naming the name.
func (r *Runner) refuseAStoreToANameWithNoParent(name string) bool {
	w := r.diag().NoParent
	if w == "" || !r.dottedNameHasNoParent(name) {
		return false
	}
	r.diagf("%s\n", Wording(w, "%[1]s: no parent", name))
	r.status = 1
	r.fatalQuiet()
	return true
}

// arithStoreWithNoParent refuses an arithmetic expression's store to a dotted
// name whose parent is missing, and reports whether it did with the error the
// evaluation stops on. Measured 2026-10-16 on ksh93u+ 2012-08-01 from a script
// file:
//
//	((a.b=1)); echo st        t.sh: line 1: a.b: no parent, ends
//	x=$((a.b=1)); echo st     the same
//	let a.b=1; echo "st=$?"   t.sh[1]: let: a.b: no parent, st=1
//	(( a.b )); echo "st=$?"   st=1 — a read asks no parent
//
// `let` speaks as a builtin and carries on, as it does over a frozen name;
// the language's own arithmetic ends the script (#6794).
func (r *Runner) arithStoreWithNoParent(name string) (bool, error) {
	if r.inBuiltin == "let" {
		refused, _ := r.refuseAnOperandWithNoParent("let", name, name)
		if !refused {
			return false, nil
		}
		r.refusedInACommand = true
		return true, errReadonlyRefusedInACommand
	}
	if !r.refuseAStoreToANameWithNoParent(name) {
		return false, nil
	}
	if r.arithCommand > 0 {
		// The command stops on the refusal it was already told about.
		return true, errReadonlyRefusedInACommand
	}
	// An expansion has nothing to report it to: the shell is already on its
	// way out, which is the whole of what the refusal costs.
	return true, nil
}
