// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// operandLocatedUnderTheCall is the name a refusal takes when the operand
// assignment that raised it ran *after* its declaration builtin had returned.
//
// The shape is a container letter over an array literal — `readonly -a a=(1)`
// — where the letter has to be recorded before the literal is stored, so the
// store is deliberately left until the builtin is finished. By then nothing is
// speaking: r.inBuiltin has been put back to whatever was running outside, and
// at the top level that is nothing at all. The dialect that carries
// Diagnostics.LiteralOperandAfterADeclarationIsLocatedUnderTheCall answers
// with the call the shell is inside instead, which is what its own store does
// with an empty command name.
//
// Second return says whether to use it, and it is false at the top level
// rather than returning the script's name: measured, the same command outside
// a function answers with no name in front of the variable at all.
//
// A function only. A sourced file is a frame too, and the dialect measured
// answers `.` there — the word the sourcing was written with, which is the
// sourcing builtin's name rather than the frame's. This shell's frames keep
// the operand and not the word, so that case is left alone rather than
// answered with something else; see the field's own note.
func (r *Runner) operandLocatedUnderTheCall() (string, bool) {
	if !r.diag().LiteralOperandAfterADeclarationIsLocatedUnderTheCall {
		return "", false
	}
	if len(r.frames) == 0 {
		return "", false
	}
	f := r.frames[len(r.frames)-1]
	if !f.IsFunction() {
		return "", false
	}
	if f.ran {
		// Only the **first** command a call runs is spoken for by the call.
		// Measured 2026-09-23 on bash 5.3.20, with `declare -air a=(1)`
		// standing and `g` the enclosing function:
		//
		//	g() { readonly -a a=(1); }              g: a: readonly variable
		//	g() { :; readonly -a a=(1); }           a: readonly variable
		//	g() { true; readonly -a a=(1); }        a: readonly variable
		//	g() { echo x >/dev/null; readonly … }   a: readonly variable
		//	g() { zz=1; readonly -a a=(1); }        a: readonly variable
		//	g() { local ok=1; readonly -a a=(1); }  a: readonly variable
		//	g() { { readonly -a a=(1); }; }         g: a: readonly variable
		//	g() { ( readonly -a a=(1) ); }          g: a: readonly variable
		//	g() { for i in 1 2; do readonly … done  g: … then a: …
		//	h() { readonly -a a=(1); }; g() { :; h; }   h: a: readonly variable
		//
		// Nine shapes and they are uniform: anything the call has already
		// dispatched takes the name away, a compound wrapper does not
		// because it is not a command, a fresh call gets its own name back,
		// and a loop's second turn has lost it. The top level never has a
		// name to give, which the length check above already answers.
		//
		// This was measured wrong the first time and shipped that way in
		// #4382: the name was given to every refusal in a call rather than
		// to the first command's. It was right on the row it was taken for —
		// `attr.tests` calls one-statement functions — and wrong everywhere
		// else.
		return "", false
	}
	return f.Name, true
}

// reportTheStoresOwnRefusal writes the refusal the *store* makes, in the
// dialect where a declaration builtin refusing an array literal lets both
// writers speak — see
// Diagnostics.LiteralOperandRefusedAlsoSpeaksForTheStore.
//
// It names no builtin, because the store is not one. It names the enclosing
// call under the same rule everything else on this path does, which is why it
// goes through operandLocatedUnderTheCall rather than reading the frame
// itself: the call speaks for the first command it runs and for nothing after
// it.
func (r *Runner) reportTheStoresOwnRefusal(name string) {
	msg := Wording(r.diag().ReadonlyVariable, "%s: readonly variable", name)
	outerBuiltin, outerCall := r.inBuiltin, r.locatedUnderACall
	// Cleared for the location: the builtin whose refusal is coming next is
	// not what is speaking here.
	r.inBuiltin, r.locatedUnderACall = "", false
	if call, under := r.operandLocatedUnderTheCall(); under && r.diag().ReadonlyVariableInDeclaration != "" {
		msg = Wording(r.diag().ReadonlyVariableInDeclaration, "", name, call)
	}
	r.diagf("%s\n", msg)
	r.inBuiltin, r.locatedUnderACall = outerBuiltin, outerCall
}

// storeSpeaksBeforeTheBuiltin reports whether this refusal is the two-writer
// shape: a builtin that names itself, refusing a name whose operand on this
// very command was an array literal.
//
// r.literalOperands is what says the operand was a literal, and it is the
// command's own record rather than anything about the name — the same table
// the declaration path already reads to tell `typeset a=(x)` from `typeset
// a=x`.
func (r *Runner) storeSpeaksBeforeTheBuiltin(form assignForm, name string) bool {
	return r.diag().LiteralOperandRefusedAlsoSpeaksForTheStore &&
		r.literalOperands[name] &&
		r.readonlyRefusalNamesBuiltin(form)
}
