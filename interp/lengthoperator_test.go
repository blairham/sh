// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `${#v}` and an operator in the same expansion. Two faults wore one shape,
// and neither said anything.
//
// The length was answered from the untouched value and the operator was
// dropped, so `v=abc; ${#v#a}` was 3 — the length of `abc` rather than of the
// `bc` the trim leaves. A script computing a trimmed length got the untrimmed
// one at status 0, and 3 is not merely a different answer from 2: it is
// nobody's, because four of the five shells refuse the expansion outright.
//
// Measured 2026-09-06. zsh 5.9.2 accepts it and measures what the operator
// leaves; bash 5.3.15, bash 3.2.57, that build invoked as `sh`, dash and
// ksh93u+ 2012-08-01 all answer `bad substitution`. The refusal is deferred in
// every one of them — `if false; then echo ${#v#a}; fi` runs clean in all five
// — which is why the grammar marks the node rather than failing while reading.

// lengthOpRun runs src with the grammar that has the pairing, and the axes
// these rows reach answered.
func lengthOpRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamLengthTakesAnOperator = true
		d.ParamSubstitution = true
		d.ParamElementSelection = true
	}, func(r *Runner) {
		sem := CoreSemantics()
		sem.ArrayScalarIsTheWholeArray = No
		// The rows naming `${a[0]}` reach the base question, which is not
		// what any of them is about.
		sem.ArrayBaseIsZero = Yes
		r.Semantics = &sem
	})
}

// Apply, then measure — uniform across every operator the grammar has. Each
// want is a different number from the 3 the bug returned, and from each other
// where the operator allows, so no row can pass by accident.
func TestTheLengthMeasuresWhatTheOperatorLeaves(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`v=abc; echo ${#v#a}`, "2"},
		{`v=abc; echo ${#v##*}`, "0"},
		{`v=abc; echo ${#v%c}`, "2"},
		{`v=abc; echo ${#v%%b*}`, "1"},
		{`v=abc; echo ${#v:1}`, "2"},
		{`v=abc; echo ${#v:1:1}`, "1"},
		{`v=abc; echo ${#v/b/XX}`, "4"},
		{`v=abc; echo ${#v//?/XX}`, "6"},
		// The conditionals: the parameter side measures the parameter and
		// the word side measures the word.
		{`v=abc; echo ${#v:-d}`, "3"},
		{`v=abc; echo ${#v:+w}`, "1"},
		{`unset v; echo ${#v:-dddd}`, "4"},
		// The element exclusion matches a whole value, so it leaves either
		// the value or nothing.
		{`v=abc; echo ${#v:#ab*}`, "0"},
		{`v=abc; echo ${#v:#zz}`, "3"},
		// A pattern that trims nothing still goes through the operator, and
		// the answer coincides with the plain length — which is why the rows
		// above are the ones that carry the assertion.
		{`v=abc; echo ${#v#z}`, "3"},
	} {
		if out, st := lengthOpRun(t, c.src); strings.TrimSpace(out) != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, strings.TrimSpace(out), st, c.want)
		}
	}
}

// The plain length is untouched, and neither is the count. `${#v}` measures
// the value, `${#a[@]}` counts elements, `${#@}` counts parameters — all
// unanimous, and none of them is the pairing.
func TestTheLengthWithoutAnOperatorIsUnchanged(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`v=abc; echo ${#v}`, "3"},
		{`a=(xx yy); echo ${#a[@]}`, "2"},
		{`a=(xx yy); echo ${#a[0]}`, "2"},
		{`set -- p qq; echo ${#@}`, "2"},
		{`set -- p qq; echo ${#*}`, "2"},
		{`unset v; echo ${#v}`, "0"},
	} {
		if out, st := lengthOpRun(t, c.src); strings.TrimSpace(out) != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, strings.TrimSpace(out), st, c.want)
		}
	}
}

// A subscript and an operator together: the subscript names the value and the
// operator applies to it, then the length measures the result.
func TestTheLengthOfASubscriptedValueUnderAnOperator(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(hello yy); echo ${#a[0]#he}`, "3"},
		{`a=(hello yy); echo ${#a[0]:2}`, "3"},
	} {
		if out, st := lengthOpRun(t, c.src); strings.TrimSpace(out) != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, strings.TrimSpace(out), st, c.want)
		}
	}
}

// The operator's operand is expanded once, which is what puts the interception
// before any value is read rather than beside the length. A command
// substitution in the pattern has side effects, and measuring the result
// afterwards must not run them a second time.
func TestTheOperandOfALengthOperatorRunsOnce(t *testing.T) {
	const src = `v=abc; f=0; echo ${#v#$(f=$((f+1)); echo count >&2; echo a)}`
	out, st := lengthOpRun(t, src)
	if st != 0 {
		t.Fatalf("status %d, want 0: %q", st, out)
	}
	if n := strings.Count(out, "count"); n != 1 {
		t.Errorf("the substitution ran %d times, want 1: %q", n, out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("got %q, want the length of what the trim left", out)
	}
}

// Without the grammar flag the pairing is not a construct at all, and the
// node the parser marks is reported as a bad substitution when the expansion
// is reached. Asserted as a nonzero status with the text named, because "not 2"
// would also pass against a silent 3.
func TestWithoutTheFlagTheLengthTakesNoOperator(t *testing.T) {
	for _, src := range []string{
		`v=abc; echo ${#v#a}`,
		`v=abc; echo ${#v:1}`,
		`v=abc; echo ${#v/b/XX}`,
		`v=abc; echo ${#v:-d}`,
	} {
		out, st := run(t, src, nil)
		if st == 0 {
			t.Errorf("%s = %q at status 0, want a refusal", src, strings.TrimSpace(out))
		}
		if !strings.Contains(out, "bad substitution") {
			t.Errorf("%s = %q, want it to name a bad substitution", src, out)
		}
	}
	// And the plain length still works there, which is the boundary: the
	// refusal is the operator's and not the length's.
	if out, st := run(t, `v=abc; echo ${#v}`, nil); strings.TrimSpace(out) != "3" || st != 0 {
		t.Errorf("${#v} without the flag = %q status %d, want 3 at 0", strings.TrimSpace(out), st)
	}
}

// The refusal is deferred, in every dialect: an expansion in a branch never
// taken is not an error at all. Measured in all five shells that refuse it —
// including the one whose grammar otherwise refuses an unknown operator while
// reading — so the node is marked rather than failed at parse time.
func TestTheRefusalIsDeferredToTheExpansion(t *testing.T) {
	const src = `v=abc; if false; then echo ${#v#a}; fi; echo reached`
	if out, st := run(t, src, nil); strings.TrimSpace(out) != "reached" || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", strings.TrimSpace(out), st, "reached")
	}
	// And it parses, which is the half a runtime test cannot see.
	if _, err := syntax.Parse(src, syntax.Core()); err != nil {
		t.Errorf("parse: %v, want the node to be built and marked rather than refused", err)
	}
}
