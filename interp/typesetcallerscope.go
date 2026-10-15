// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// declareInTheKeywordCallersScope makes a declaration running in a function
// without the `function` word see the scope of the nearest keyword-defined
// call below it as the innermost one, in the dialect where only such a call has
// a scope (Semantics.TypesetLocalNeedsKeywordFunction). The returned function
// puts the stack back, and is to be deferred.
//
// The model is that a POSIX-style function has no variable scope of its own
// there and runs in its caller's: measured 2026-10-15 on ksh93u+ 2012-08-01,
// `-c` under `env -i PATH=/usr/bin:/bin`,
//
//	function g { h; echo "g=${x-u}"; }; h() { typeset x=5; }; g; echo "top=${x-u}"
//	                                        g=5, top=u
//	function g { k; echo "g=${x-u}"; }; k() { h; }; h() { typeset x=5; }; g
//	                                        g=5 — through two such calls
//	h() { typeset x=5; }; h; echo "top=${x-u}"
//	                                        top=5 — no keyword call below
//
// and the freeze of `readonly` takes the same scope, since it is declared
// where `typeset` declares (#6488). So the declaration does exactly what it
// would have done written in g's own body, which is what hiding the scopes
// above g says — every check a declaration makes of "the scope it is in",
// the shadow itself and the refusals in front of it, then reads g's.
//
// **Read rather than asked**, for the reason keywordGatesALocalScope gives: a
// dialect that has not answered keeps the stack whole and meets the question
// in shadowTypeset as before, and with no keyword call below there is nothing
// to lend — shadowTypeset's own answer, no binding, stands.
//
// The stack is cut with its capacity, so a call the declaration runs (a
// discipline function) appends to a fresh array rather than over the hidden
// scopes.
func (r *Runner) declareInTheKeywordCallersScope() func() {
	n := len(r.scopes)
	if n == 0 || r.scopes[n-1].keyword || !r.keywordGatesALocalScope() {
		return func() {}
	}
	for i := n - 2; i >= 0; i-- {
		if r.scopes[i].keyword {
			saved := r.scopes
			r.scopes = saved[: i+1 : i+1]
			return func() { r.scopes = saved }
		}
	}
	return func() {}
}
