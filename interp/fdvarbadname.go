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
	}
	// A dotted name is one where no two dots meet and there is more than a
	// dot: `{a..b}`, `{..a}` and `{.}` are `invalid variable name` in
	// ksh93u+, where `{.a}` is taken. Whether a dotted name's parent exists
	// is a later question, and not this one.
	return isPlainName(base) ||
		(r.dottedName(base) && base != "." && !strings.Contains(base, ".."))
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
