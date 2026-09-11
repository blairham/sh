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

// lengthOpListRun is lengthOpRun on the other side of the array axes: a bare
// name that *is* its elements, subscripts counting from one and a comma read
// as a range. That is the grammar the rows below belong to — the pairing of a
// length with an operator exists in one shell, and it is the one that reads an
// array this way.
func lengthOpListRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamLengthTakesAnOperator = true
		d.ParamSubstitution = true
		d.ParamElementSelection = true
	}, func(r *Runner) {
		sem := CoreSemantics()
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		sem.ArrayLengthWithoutSubscriptIsCount = Yes
		sem.WholeSubscriptOnAScalarMeasuresIt = Yes
		sem.ArrayBaseIsZero = No
		sem.SubscriptCommaIsARange = Yes
		sem.SplitParamExpansion = No
		r.Semantics = &sem
	})
}

// The length over an operator counts what the operator *left*, where what it
// left is a list.
//
// The measurement is uniform — apply, then measure — and the half that was
// missing is which measurement: this took the width of the joined value for
// every list, so `${#a:#one}` on a three-element array was 13, the length of
// `one two three`, rather than the 2 elements the filter leaves. A plausible
// number at status 0, and the wrong one for `(( ${#list:#$x} ))`, which is how
// a script asks whether a name is in a list (#1651).
//
// Named for the axes rather than for a shell. Measured on zsh 5.9.2, the only
// shell that builds this node; the corpus row is
// `param/length-over-an-operator-counts-what-it-left`.
func TestTheLengthOverAnOperatorCountsWhatItLeft(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(one two three); echo ${#a:#one}`, "2"},
		{`a=(one two three); echo ${#a:#*}`, "0"},
		{`a=(one two three); echo ${#a#o}`, "3"},
		{`a=(one two three); echo ${#a%%e}`, "3"},
		{`a=(one two three); echo ${#a//e/E}`, "3"},
		{`a=(one two three); echo ${#a:-zz}`, "3"},
		{`a=(one two three); echo ${#a[@]:#one}`, "2"},
		{`a=(one two three); echo ${#a[*]:#one}`, "2"},
		{`a=(one two three); echo ${#a[1,3]:#one}`, "2"},
		{`a=(one two three); b=(two four); echo ${#a:|b}`, "2"},
		{`a=(o two); echo ${#a#o}`, "2"},
		{`set -- p q r; echo ${#@:#q}`, "2"},
		// An element the trim emptied is still an element. The command-line
		// reading drops it, which would answer 2.
		{`set -- p q r; echo ${#@#p}`, "3"},

		// The other half of the rule: what the operator left is one string,
		// so the length is its width. Beside the rows above rather than in a
		// test of their own, because they are the readings those must not
		// give.
		{`a=(one two three); echo ${#a[1]#o}`, "2"},
		{`s="one two three"; echo ${#s:#one}`, "13"},
		{`a=(one two three); unset u; echo ${#u:-$a}`, "13"},
		{`a=(one two three); s=""; echo ${#s:-$a}`, "13"},

		// The controls, which were already right: no operator at all, and a
		// scalar under one.
		{`a=(one two three); echo ${#a}`, "3"},
		{`v=abc; echo ${#v#a}`, "2"},
	} {
		if out, st := lengthOpListRun(t, c.src); strings.TrimSpace(out) != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, strings.TrimSpace(out), st, c.want)
		}
	}
}

// `${#s[@]}` on a name holding one string: one dialect measures the string,
// the others count a list of one.
//
// The three-character row is what says which reading it is. An empty scalar
// answering 0 on its own would only say "no elements"; `a b` answering 3 says
// the whole-array subscript reached the *value*. This is how a script asks
// "did I get anything?" after a parse, and 1 for a name that never became an
// array is a count agreeing with the wrong answer (#1553).
//
// Both answers are asserted, which is what makes it an axis rather than a
// fix: the rows are the same script under the two readings.
func TestAWholeSubscriptOnAScalarMeasuresTheValueOrCountsOne(t *testing.T) {
	for _, c := range []struct {
		src           string
		measures, one string
	}{
		{`a=""; echo ${#a[@]}`, "0", "1"},
		{`a=""; echo ${#a[*]}`, "0", "1"},
		{`b=x; echo ${#b[@]}`, "1", "1"},
		{`h="a b"; echo ${#h[@]}`, "3", "1"},
		// A scalar with an operator on it asks the same question, because
		// the count and the width are the same pair either way.
		{`h="a b"; echo ${#h[@]:#zz}`, "3", "1"},

		// The rows that ask nobody: an array is a list under both readings,
		// and an unset name is nothing under both.
		{`a=(xx yy); echo ${#a[@]}`, "2", "2"},
		{`unset a; echo ${#a[@]}`, "0", "0"},
		{`h="a b"; echo ${#h}`, "3", "3"},
	} {
		for _, side := range []struct {
			name   string
			answer Answer
			want   string
		}{
			{"measuring the value", Yes, c.measures},
			{"counting a list of one", No, c.one},
		} {
			run := func(r *Runner) {
				sem := CoreSemantics()
				sem.ArrayScalarIsTheWholeArray = No
				sem.ArrayBaseIsZero = Yes
				sem.WholeSubscriptOnAScalarMeasuresIt = side.answer
				r.Semantics = &sem
			}
			out, st := runGrammar(t, c.src, func(d *syntax.Dialect) {
				d.ParamLengthTakesAnOperator = true
				d.ParamElementSelection = true
			}, run)
			if strings.TrimSpace(out) != side.want || st != 0 {
				t.Errorf("%s with %s = %q status %d, want %q at 0",
					c.src, side.name, strings.TrimSpace(out), st, side.want)
			}
		}
	}
}
