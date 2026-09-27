// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The fourth produced-parameter seam, and the only one that produces nothing.
//
// A dialect with modules has to be able to say "this shell has not got that
// parameter" in a way a *script* can hear. Without it the only available
// answer was an empty value at status 0, which is indistinguishable from a
// real answer — and the whole reason a module could not load over a parameter
// nobody in the file touched (#1058, #1146).

// runAbsent runs one line and gives back what went to each stream.
func runAbsent(t *testing.T, src string) (out, errs string, status int) {
	t.Helper()
	var o, e strings.Builder
	r := seamRunner(t, &o, &e)
	r.SetAbsentParameter("nothere", "parameter not implemented yet")
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return o.String(), e.String(), r.ExitStatus()
}

// **Every route that reads a parameter refuses, and each one names it.**
//
// Written route by route because the routes do not share a bottom. The
// subscript one is the reason this is a table and not a single assertion:
// `${m[k]}` on a name nothing holds is answered by the array path with no
// fields at all, so a test placed where a value is fetched never sees it —
// and `${m[k]}` is the spelling a caller of a parameter module writes most.
//
// Each row asserts the *name* is in the diagnostic and that nothing reached
// standard output. A row that checked only the status would pass against a
// shell that expanded to nothing quietly, which is the exact bug this seam
// exists to prevent.
func TestAnAbsentParameterRefusesByNameOnEveryRoute(t *testing.T) {
	for _, src := range []string{
		`printf '[%s]' "$nothere"`,
		`printf '[%s]' "${nothere}"`,
		`printf '[%s]' "${nothere[k]}"`,
		`printf '[%s]' "${nothere[@]}"`,
		`printf '[%s]' "${#nothere}"`,
		`printf '[%s]' ${nothere[k]}`,
		`x="${nothere[k]}"; printf '[%s]' "$x"`,
	} {
		t.Run(src, func(t *testing.T) {
			out, errs, status := runAbsent(t, src)
			if !strings.Contains(errs, "nothere: parameter not implemented yet") {
				t.Errorf("%s: stderr = %q, want the parameter named", src, errs)
			}
			if out != "" {
				t.Errorf("%s: stdout = %q, want nothing handed to the caller", src, out)
			}
			if status == 0 {
				t.Errorf("%s: status 0, want a failure", src)
			}
		})
	}
}

// It is a refusal to read something absent and not a claim on the spelling: a
// script that gave the name a value of its own is answered with it, and the
// conditional operators — which are a script saying what to do when there is
// no value — are left alone, exactly as `set -u` leaves them.
func TestAnAbsentParameterYieldsToTheScriptsOwnAnswer(t *testing.T) {
	out, errs, status := runAbsent(t, `printf '[%s]' "${nothere-d}" "${nothere+set}"
nothere=mine
printf '[%s]' "$nothere" "${#nothere}"`)
	if want := "[d][][mine][4]"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errs != "" || status != 0 {
		t.Errorf("stderr = %q (status %d), want silence and 0", errs, status)
	}
}

// **A name that refuses is not a name the shell has.** The two questions a
// module loader asks are different ones, and answering the first with the
// second is how "refuses legibly" would quietly become "implemented" — which
// would let a module load over a parameter that reads empty, undoing the rule
// this seam exists to serve.
func TestAnAbsentParameterIsNotADynamicOne(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetAbsentParameter("nothere", "parameter not implemented yet")
	r.SetDynamicAssoc("real", func(*Runner) AssocArray { return AssocArray{"k": Scalar("v")} })
	if r.DynamicParameter("nothere") {
		t.Error("DynamicParameter(nothere) = true, want false — nothing produces it")
	}
	if !r.AbsentParameter("nothere") {
		t.Error("AbsentParameter(nothere) = false, want true")
	}
	if !r.DynamicParameter("real") || r.AbsentParameter("real") {
		t.Error("a produced parameter answered the absent question, or failed the produced one")
	}
	if r.AbsentParameter("neither") || r.DynamicParameter("neither") {
		t.Error("a name nobody registered answered one of the two questions")
	}
}

// **`unset "nothere[k]"` refuses too**, and that route is the write half the
// seam was missing (#1527).
//
// It reached nothing above because an `unset` operand is not an expansion:
// no word is expanded, no value is fetched, and the two places
// refuseAbsentParameter is asked from are both in the word paths. What the
// operand reached instead was the ordinary subscript machinery, which reads
// the brackets of a name holding nothing as *arithmetic* — so the two keys
// below used to split, and neither answer was about the parameter. A key whose
// name is an unset variable evaluated to 0 and the unset was **silent at
// status 0**; a key that happened to name a variable holding text complained
// about that text not being a number, which is a sentence about the wrong
// thing entirely.
//
// Both keys are here for that reason: a test with only one of them passes
// against a fix that reads the subscript first.
func TestAnAbsentParameterRefusesAnUnsetOfAnElement(t *testing.T) {
	for _, src := range []string{
		`unset "nothere[k]"`,
		`unset 'nothere[somepath]'`,
		`unset "nothere[1]"`,
		`unset "nothere[1,2]"`,
		`unset "nothere[@]"`,
	} {
		out, errs, status := runAbsentWithVar(t, src, "somepath", "/usr/local/bin")
		if !strings.Contains(errs, "nothere: parameter not implemented yet") {
			t.Errorf("%s: stderr = %q, want the parameter named", src, errs)
		}
		if status != 1 {
			t.Errorf("%s: status = %d, want 1", src, status)
		}
		if out != "" {
			t.Errorf("%s: stdout = %q, want nothing", src, out)
		}
	}
}

// And the script's own value is the script's on this route as on the read: a
// name it assigned is an ordinary one and `unset` reaches into it by whatever
// rule the axes give — here a character of a scalar, so `one` becomes `oe`.
// The result is asserted rather than the silence, because silence alone would
// also be what a refusal that forgot to write its sentence looks like. Without
// this the refusal would be about owning a spelling rather than about reading
// something absent.
func TestUnsettingAnElementOfAnAbsentNameTheScriptOwnsIsOrdinary(t *testing.T) {
	out, errs, status := runAbsentWithVar(t, `nothere=one
unset "nothere[1]"
printf '[%s]' "${nothere-gone}"`, "unused", "")
	if want := "[oe]"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errs != "" || status != 0 {
		t.Errorf("stderr = %q (status %d), want silence and 0", errs, status)
	}
}

// runAbsentWithVar is runAbsent with one ordinary variable set, so a subscript
// naming it evaluates to that text rather than to zero — which is what tells
// the two old answers apart.
func runAbsentWithVar(t *testing.T, src, name, value string) (out, errs string, status int) {
	t.Helper()
	var o, e strings.Builder
	r := seamRunner(t, &o, &e)
	// The one axis these two tests need: a core with none answered refuses
	// `unset a[k]` as a spelling the shells disagree about, several steps
	// ahead of anything this seam decides, so an unanswered axis would report
	// itself and the test would assert on the wrong diagnostic.
	sem := Semantics{
		FatalErrorStatusIsOne:       Yes,
		UnsetTakesASubscript:        Yes,
		ArrayBaseIsZero:             Yes,
		ScalarSubscriptIsACharacter: Yes,
	}
	r.Semantics = &sem
	r.SetAbsentParameter("nothere", "parameter not implemented yet")
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	r.Vars[name] = value
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return o.String(), e.String(), r.ExitStatus()
}

// runAbsentSetTest is runAbsent with the one grammar flag the set test needs.
//
// Named for the flag and not for a shell, which is this package's rule: what
// the rows below depend on is `${+name}` being *parsed* as a set test, and
// that is [syntax.Dialect.ParamSetTestFlag] rather than any particular shell
// having it.
func runAbsentSetTest(t *testing.T, src string) (out, errs string, status int) {
	t.Helper()
	var o, e strings.Builder
	r := seamRunner(t, &o, &e)
	r.SetAbsentParameter("nothere", "parameter not implemented yet")
	d := syntax.Core()
	d.ParamSetTestFlag = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return o.String(), e.String(), r.ExitStatus()
}

// The set test is the same question the conditional operators ask, in the
// spelling that is not an operator at all — so it is answered rather than
// refused.
//
// `${+name}` substitutes `1` or `0` and never the value, so there is nothing
// for an absent name to read as empty, which is the whole of what this seam
// guards. It refused for two years while `${name+x}` beside it answered, and
// the cost was not a wrong answer: `(( ${+name} ))` is what a script writes
// *before* depending on a parameter, and it got no further lines (#4882).
//
// A subscript and an expansion flag leave the reading alone and a written
// operator takes it away, which is measured rather than read off the flag —
// see refuseAbsentParameter for the reference's rows.
func TestTheSetTestOfAnAbsentParameterIsAnswered(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf '[%s]' "${+nothere}"`, "[0]"},
		// A subscript does not make it a value read.
		{`printf '[%s]' "${+nothere[k]}"`, "[0]"},
		// And the answer is a real one, from the same runner: a name that
		// *is* there answers 1, so `0` above is the test working rather than
		// a constant.
		{`there=1; printf '[%s]' "${+there}" "${+nothere}" "${+neverheardof}"`, "[1][0][0]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, errs, status := runAbsentSetTest(t, tc.src)
			if out != tc.want {
				t.Errorf("stdout = %q, want %q", out, tc.want)
			}
			if errs != "" || status != 0 {
				t.Errorf("stderr = %q (status %d), want silence and 0", errs, status)
			}
		})
	}
}

// And the exemption is the *pure* set test, which is the control that says it
// was widened by one spelling rather than switched off.
//
// Each of these carries the `+` and reads a value through an operator or a
// length, so each still names the parameter and still fails.
func TestASetTestWithAnOperatorStillRefuses(t *testing.T) {
	for _, src := range []string{
		`printf '[%s]' "${+nothere#a}"`,
		`printf '[%s]' "${+nothere%a}"`,
		`printf '[%s]' "${#nothere}"`,
		`printf '[%s]' "$nothere"`,
	} {
		t.Run(src, func(t *testing.T) {
			out, errs, status := runAbsentSetTest(t, src)
			if !strings.Contains(errs, "nothere: parameter not implemented yet") {
				t.Errorf("stderr = %q, want the parameter named", errs)
			}
			if out != "" {
				t.Errorf("stdout = %q, want nothing handed to the caller", out)
			}
			if status == 0 {
				t.Error("status 0, want a failure")
			}
		})
	}
}
