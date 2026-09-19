// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The shell that carries every arithmetic value in a C double writes one as an
// integer whenever a saturating `(intmax_t)` cast of it converts back to the
// same double. Whether a float is written that way is therefore
// Semantics.ArithValuesAreCarriedInAFloat's to answer — and it must be asked
// **only where the two readings disagree**.
//
// That guard is not decoration. A shell with floats and no answer for the axis
// is exactly the shape #2272 is about: an axis added after a dialect is
// written holds Unspecified there and refuses at run time, in the shipped
// binary, with every test green. Asking on every float would put `$(( 2.5e10 ))`
// — whose two readings are the same five bytes — behind an axis the dialect
// never had to answer.
//
// Both rows below are one runner with the axis unanswered and floats in the
// grammar. Removing the guard passes the first and fails nothing else, which is
// why it is written down here rather than left to a sweep.
func TestTheIntegerRenderingIsAskedOnlyWhereTheReadingsDisagree(t *testing.T) {
	floats := func(d *syntax.Dialect) { d.ArithFloat = true }
	unanswered := func(r *Runner) {
		sem := CoreSemantics()
		sem.ArithValuesAreCarriedInAFloat = Unspecified
		dg := Diagnostics{ArithFloatDigits: 15}
		r.Semantics, r.Diagnostics = &sem, &dg
	}

	// The two readings agree — `%g` and the cast both write `25000000000` —
	// so the axis is not a question and the script runs on.
	out, st := runGrammar(t, "echo $((2.5e10))\necho after\n", floats, unanswered)
	if st != 0 || strings.TrimSpace(out) != "25000000000\nafter" {
		t.Errorf("a float both readings write alike: out=%q st=%d, want it written and the script continuing", out, st)
	}

	// And where they disagree the axis really is asked, and an unanswered one
	// says so — which is what makes the guard above a narrowing rather than a
	// silence. The assertion is on the sentence rather than on the status:
	// this refusal is reported from inside an expansion that has already
	// produced its text, and what a shell with no dialect then exits is a
	// question of its own and not this one's.
	out, _ = runGrammar(t, "echo $((1e19/3))\n", floats, unanswered)
	if !strings.Contains(out, "arithmetic carried in a float rather than the machine word") {
		t.Errorf("a float the two readings write differently: out=%q, want the unanswered axis named", out)
	}
}
