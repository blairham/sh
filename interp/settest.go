// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// The `${+name}` flag: a `+` written between the `${` and the parameter,
// which asks whether the parameter is set and substitutes `1` or `0` instead
// of its value. One dialect's grammar has it — see
// [syntax.Dialect.ParamSetTestFlag] — and the answer is here because it is
// the same question every conditional expansion already asks, from the same
// source, so the two cannot come to different answers.
//
// The construct **never fails**, which is the whole reason a script reaches
// for it: it is the guard in front of everything else, and a shell that
// raises an error where the answer is `0` turns every guard into a fatal
// stop rather than a false.

// setTestAnswers reports whether this expansion is that question rather than
// an ordinary substitution.
//
// The `+` is read by the grammar wherever the dialect has it, but it is the
// answer only when no operator was written to answer instead. Measured:
// `v=abc; ${+v#a}` is `bc`, `${+v:-x}` on an unset `v` is `x`, and
// `${+v=w}` assigns — so an operator takes the expansion back to exactly
// what it would have been without the `+`, rather than being refused.
//
// A length cannot reach this and needs no clause: `${+#v}` is refused while
// reading, and `${#+v}` never carries the flag at all.
func setTestAnswers(e *syntax.ParamExpr) bool {
	return e.SetTest && e.Op == syntax.ParamNone
}

// setTestResult is the substitution the question makes.
//
// Set-ness is not emptiness, and the two are what this construct exists to
// tell apart: `v=; ${+v}` is `1` where `${v:+x}` yields nothing, and a name
// that was assigned and then unset is `0`. It is one field, whatever the
// parameter holds — an array's element count does not reach it.
func setTestResult(set bool) string {
	if set {
		return "1"
	}
	return "0"
}
