// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A `{word}` redirection whose word is not a name, which two grammars lex:
// ksh93 takes any brace word before a redirection operator, and zsh a
// digit-led one. What the two shells then do with it is
// Semantics.FdVariableBadName, and the measurements are there (#6522).

// fdVarNameIsAName reports whether the text between a `{name}`'s braces is
// something a descriptor can be stored in: a name, an element of one, a
// positional parameter where the grammar has them there, or a dotted name
// where the grammar has those.
func (r *Runner) fdVarNameIsAName(ref string) bool {
	if _, ok := positionalFdVar(ref); ok {
		return r.lang().FdVariablePositional
	}
	base := ref
	if i := strings.IndexByte(ref, '['); i > 0 && strings.HasSuffix(ref, "]") {
		base = ref[:i]
	} else if i > 0 && !strings.Contains(ref, "]") && r.lang().FdVariableAnyBraceWord {
		// A subscript that never closes is no subscript: the name before
		// the bracket is stored into. See fdVarStoredName.
		base = ref[:i]
	}
	// A dotted name is one where no two dots meet and there is more than a
	// dot: `{a..b}`, `{..a}` and `{.}` are `invalid variable name` in
	// ksh93u+, where `{.a}` is taken. Whether a dotted name's parent exists
	// is a later question, and not this one. A letter past ASCII is a name
	// where the locale says it is one, as it is in an assignment: `{é}` is
	// taken under a UTF-8 locale and refused under C (#6591).
	return isPlainName(base) || r.isLocaleName(base) ||
		(r.dottedName(base) && base != "." && !strings.Contains(base, ".."))
}

// fdVarStoredName is the name a `{word}` redirection's number goes into,
// which is the word itself except in the grammar that takes any brace word:
// there a subscript that never closes is dropped, and the name in front of
// the bracket is what receives the number. Measured 2026-10-15 on ksh93u+
// 2012-08-01, `-c`, `echo {W}</dev/null; echo "st=$? ${a[@]}"`: `{a[}`,
// `{a[1}` and `{a[x}` all store 10 into `a` at 0, and `{a[}</nonexist` is
// the open's failure rather than a refusal (#6591).
//
// Not modeled: over an array that already holds elements, ksh93u+ leaves
// the character codes of their old values behind (`a=(1 2); echo
// {a[1}</dev/null` leaves `49 10`), which reads as the integer retyping of
// FdVariableStoreTyping applied to the text byte by byte.
func (r *Runner) fdVarStoredName(ref string) string {
	if !r.lang().FdVariableAnyBraceWord {
		return ref
	}
	if i := strings.IndexByte(ref, '['); i > 0 && !strings.Contains(ref, "]") {
		return ref[:i]
	}
	return ref
}

// fdVarBadNamePolicy asks the axis, and only of a word that is not a name.
func (r *Runner) fdVarBadNamePolicy(ref string) FdVariableBadNamePolicy {
	if r.fdVarNameIsAName(ref) {
		return FdVariableBadNameUnspecified
	}
	p := r.sem().FdVariableBadName
	if p == FdVariableBadNameUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("what a `{word}` redirection whose word is not a name comes to")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// fdVarBadNameFailsTheRedirection refuses the word ahead of everything else
// about the redirection, in the dialect that does that, and reports whether
// the redirection is over. The refusal is a failed redirection's: status 1,
// the command not run, and a special builtin's made fatal where that rule is
// kept — all of which the caller's redirErr already carries.
func (r *Runner) fdVarBadNameFailsTheRedirection(ref string) bool {
	if r.fdVarNameIsAName(ref) {
		return r.sem().FdVariableBadName == FdVariableBadNameFailsTheRedirection &&
			r.fdVarNameFailsFirst(ref)
	}
	switch r.fdVarBadNamePolicy(ref) {
	case FdVariableBadNameFailsTheRedirection:
		r.refuseFdVarName(ref)
		r.status = 1
		return true
	case FdVariableBadNameUnspecified:
		return r.unspecified
	}
	return false
}

// fdVarBadNameRefusedAtTheStore refuses the word at the store, in the
// dialect that does that, and reports whether it did.
//
// Fatal to the shell the store is made in. On a command that is a process
// of its own the store is that process's, so what ends is the command and
// not this shell: the status is 1 and the next command runs, which is the
// row `/usr/bin/true {1a}</dev/null; echo st=$?` measures.
func (r *Runner) fdVarBadNameRefusedAtTheStore(ref string) bool {
	if r.fdVarNameIsAName(ref) || r.sem().FdVariableBadName != FdVariableBadNameIsAFatalStoreError {
		return false
	}
	r.refuseFdVarName(ref)
	if r.redirOwner == redirOwnerTheCommand {
		r.status = 1
		return true
	}
	r.fatalQuiet()
	return true
}

// refuseFdVarName writes the refusal.
func (r *Runner) refuseFdVarName(ref string) {
	r.diagf("%s\n", Wording(r.diag().FdVariableBadName, "%[1]s: invalid variable name", ref))
}

// fdVarNameFailsFirst refuses, in the dialect that settles a `{word}` before
// anything else about its redirection, a name that is a name but cannot be
// stored into, and reports whether it did. Two ways, measured 2026-10-15 on
// ksh93u+ 2012-08-01 with `-c` (#6591):
//
//	echo {a.b}</dev/null; echo "st=$?"           a.b: no parent, st=1
//	echo {a.b}</nonexist                         a.b: no parent (no open)
//	echo {a.b}>&-, {a.b}<<<x, {a.b}<&0           a.b: no parent, st=1
//	a=1; echo {a.b.c}</dev/null                  a.b.c: no parent
//	typeset -C a; echo {a.b.c.d}</dev/null       st=0, stored
//	echo {.sh.version.x}</dev/null               st=0, stored
//	a=(1 2); i=1; true {a[$i]}</dev/null         $i: arithmetic syntax error,
//	                                             st=1, ${a[1]} still 2
//	a=(1 2); true {a[$i]}</nonexist              $i: arithmetic … (no open)
//	a=(1 2); true {a[x+]}</dev/null              x+: more tokens expected
//
// A dotted name needs its parent — everything before its last dot — to be
// something the shell holds, unless a compound variable stands above it,
// under which any path is taken. A subscript is read as it was written, so
// the `$` in `{a[$i]}` reaches the arithmetic as a character. Both are a
// failed redirection's refusal, the same as a word that is not a name: the
// command is not run, the status is 1, and `exec`'s ends the script.
func (r *Runner) fdVarNameFailsFirst(ref string) bool {
	name := r.fdVarStoredName(ref)
	if w := r.diag().FdVariableNoParent; w != "" && r.fdVarHasNoParent(name) {
		r.diagf("%s\n", Wording(w, "%[1]s: no parent", ref))
		r.status = 1
		return true
	}
	base, sub, ok := r.subscriptOperand(name)
	if !ok || r.assocDeclared(base) || wholeArraySubscript(sub) {
		return false
	}
	if _, err := r.subscriptValueOfReference(sub); err != nil {
		r.diagf("%s\n", r.expressionFailure(sub, err))
		r.status = 1
		return true
	}
	return false
}

// fdVarHasNoParent reports whether a dotted name's parent is missing: the
// name before its last dot is nothing the shell holds, and no compound
// variable stands above it. A name with no dot past its first byte has no
// parent to miss.
func (r *Runner) fdVarHasNoParent(name string) bool {
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
	// `.sh` is the shell's own namespace and is always there, though nothing
	// is stored under the name itself: `{.sh.x}` is taken where `{.sh.q.r}`
	// is `no parent`, measured.
	if parent == ".sh" || r.namerefMemberParentIsThere(parent) {
		return false
	}
	for j := 1; j < len(parent); j++ {
		if parent[j] == '.' && r.isCompoundVariable(parent[:j]) {
			return false
		}
	}
	return true
}
