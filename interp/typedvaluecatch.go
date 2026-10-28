// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A value a typed name will not take, met by a builtin filling in one of its
// names rather than by an assignment.
//
// An assignment that will not evaluate ends the shell, in every column that
// has a typed name. Two builtins write a name the script handed them, and
// one dialect lets each of them decide what the failure costs instead.
// Measured 2026-10-15, each line followed by `echo "[$?] [$n]"; echo after`:
//
//	                         bash 5.3, 3.2   zsh 5.9.2    ksh93u+
//	typeset -i n=5;
//	  read n <<< 1+          script ends     script ends  `read: 1+: more
//	                                                      tokens expected`,
//	                                                      [1] [5], after
//	typeset -i n=5;
//	  getopts x n -y         script ends     [0] [0]      `?: arithmetic
//	                                                      syntax error`,
//	                                                      [2] [5], after
//	typeset -i n=5;
//	  getopts x n            script ends     [1] [5]      the same sentence,
//	                                                      and the script ends
//
// ksh93's `read` gives up at the name and reports 1, leaving it and every name
// after it as they were. Its `getopts` refuses the name exactly as it refuses
// a frozen one: status 2 where the scan found something, the end of the
// script where it had run out — which is GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal
// answered once for both. zsh's `getopts` never fails here, since `?` reads
// as a value there.

// typedValueCatch is one builtin store's failure, taken rather than raised.
type typedValueCatch struct {
	sentence string
	caught   bool
}

// catchingATypedValueFailure runs store with a typed value's failure taken
// rather than raised, and reports what it would have said. Nothing has been
// written or reported where it reports a failure: the caller decides both.
func (r *Runner) catchingATypedValueFailure(store func()) (string, bool) {
	outer := r.typedValueCatch
	c := &typedValueCatch{}
	r.typedValueCatch = c
	defer func() { r.typedValueCatch = outer }()
	store()
	return c.sentence, c.caught
}

// carriesATypedValue reports whether a store into name is folded through a
// numeric attribute, which is the only store that can fail this way.
func (r *Runner) carriesATypedValue(name string) bool {
	if r.attrHas(name, attrInteger) {
		return true
	}
	_, float := r.floatPrecision[name]
	return float
}

// readStoreCatchingATypedValueFailure is `read`'s store of one name. ended
// says the builtin gives up here, with st its status, because the name is
// typed and the value would not evaluate. See
// Semantics.ReadTypedValueFailureIsAnAssignmentFailure.
func (r *Runner) readStoreCatchingATypedValueFailure(name, value string) (st int, refused, ended bool) {
	if !r.carriesATypedValue(r.frozenReadName(name)) {
		st, refused = r.storeThroughOperand(name, value)
		return st, refused, false
	}
	sentence, failed := r.catchingATypedValueFailure(func() {
		st, refused = r.storeThroughOperand(name, value)
	})
	if !failed {
		return st, refused, false
	}
	if r.ask(r.sem().ReadTypedValueFailureIsAnAssignmentFailure,
		"`read` failing as an assignment does over a value its typed name will not take") {
		r.typedValueFailed("%s", sentence)
		return r.status, false, true
	}
	r.mathWrite(r.diagf, "%s", sentence)
	return 1, false, true
}

// getoptsTypedValueRefused is `getopts` meeting a value its typed name will
// not take, and reports whether the scan may carry on as if the write had
// been made, which is getoptsWrite's own question. See
// Semantics.GetoptsTypedValueFailureIsAFreeze.
func (r *Runner) getoptsTypedValueRefused(name, sentence string) bool {
	if !r.ask(r.sem().GetoptsTypedValueFailureIsAFreeze,
		"`getopts` refusing a value its typed name will not take as it refuses a frozen name") {
		// What an assignment's failure costs, which is what this was
		// before the failure was caught — and the scan goes on as it did.
		r.typedValueFailed("%s", sentence)
		return true
	}
	defer r.getoptsSpeaksAsTheScript()()
	r.reportFreezeRefusal(name, assignedByBuiltin, r.getoptsRefusalIsFatal(), sentence)
	return false
}

// getoptsSpeaksAsTheScript puts `$0` where the builtin's name goes in a
// refusal of one of `getopts`'s names, in the dialect that writes it there,
// and returns what puts the speaker back. See
// Diagnostics.GetoptsRefusalNamesTheScript.
func (r *Runner) getoptsSpeaksAsTheScript() func() {
	if !r.diag().GetoptsRefusalNamesTheScript {
		return func() {}
	}
	zero, _ := r.dollarZero()
	if d := r.diag(); d.BorrowedTextRendersTheCallStack && r.callStackHasAComponent(d) {
		// The innermost frame of the chain the location writes, which is a
		// `function`-form call's name — `$0` there anyway — or `eval` or `.`
		// where the refusal is in borrowed text. Measured 2026-10-15 on
		// ksh93u+, with `readonly o`: `eval "getopts x o -x"` is `eval[1]:
		// eval: o: is read only`, the same inside `function f` is `f[1]:
		// eval[1]: eval: …`, a `.` inside one is `f[1]: .[1]: .: …`, and an
		// `eval g` of a POSIX-form `g` names `eval`, since that call is no
		// frame of the chain (#6632).
		_, zero = r.borrowedStack(d, zero)
	}
	outer := r.builtinSpokenAs
	r.builtinSpokenAs = zero
	return func() { r.builtinSpokenAs = outer }
}
