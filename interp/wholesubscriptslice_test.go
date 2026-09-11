// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// sliceOnAScalarRun runs one snippet with the grammar a subscripted slice
// needs and one answer for the axis under test.
//
// Everything else is held still on purpose. The offsets count from zero, a
// plain name is not the whole array, and nothing splits — so the only thing
// that can move a row is the reading the axis chooses.
func sliceOnAScalarRun(t *testing.T, src string, answer Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ParamSubstring = true
	}, func(r *Runner) {
		sem := CoreSemantics()
		sem.ArrayBaseIsZero = Yes
		sem.ArrayScalarIsTheWholeArray = No
		sem.SplitParamExpansion = No
		sem.SubstringNegativeLengthIsEmpty = No
		sem.WholeSubscriptOnAScalarSlicesIt = answer
		r.Semantics = &sem
	})
}

// `${s[@]:off:len}` on a name holding one string: one reading slices the
// *characters* of that string, the other slices a list whose only element is
// the whole value.
//
// The list reading is the silent one, which is why the rows are worth having
// twice: an offset inside the value answers with the whole of it, and an
// offset of 1 drops the only element and answers nothing at all — both at
// status 0, so nothing in a script says the slice never happened (#1850).
//
// Both answers are asserted, which is what makes this an axis rather than a
// fix: the rows are the same script under the two readings.
func TestAWholeSubscriptOnAScalarSlicesTheValueOrAListOfOne(t *testing.T) {
	for _, c := range []struct {
		src              string
		characters, list string
	}{
		{`h="a b"; echo "${h[@]:0:1}"`, "a", "a b"},
		{`h="a b"; echo "${h[*]:0:1}"`, "a", "a b"},
		{`h="a b"; echo "[${h[@]:1}]"`, "[ b]", "[]"},
		{`h=abcdef; echo "${h[@]:2:3}"`, "cde", ""},

		// The offsets count characters, so the readings the scalar path
		// already carries arrive with them rather than being written out
		// again beside the list slice: a negative offset counts from the
		// end and a negative length stops short of it.
		{`h=abcdef; echo "${h[@]:(-2)}"`, "ef", "abcdef"},
		{`h=abcdef; echo "${h[@]:1:-2}"`, "bcd", ""},

		// The rows that ask nobody: a list is a list under both readings,
		// and an unset name is nothing under both.
		{`a=(x y z); echo "${a[@]:0:2}"`, "x y", "x y"},
		{`a=(x y z); echo "${a[@]:1}"`, "y z", "y z"},
		{`unset u; echo "[${u[@]:0:1}]"`, "[]", "[]"},

		// The control that says the character reading is not new. A plain
		// `${h:0:1}` is the same slice of the same value, and neither
		// reading touches it.
		{`h="a b"; echo "${h:0:1}"`, "a", "a"},
	} {
		for _, side := range []struct {
			name   string
			answer Answer
			want   string
		}{
			{"slicing the value", Yes, c.characters},
			{"slicing a list of one", No, c.list},
		} {
			out, st := sliceOnAScalarRun(t, c.src, side.answer)
			if strings.TrimSpace(out) != side.want || st != 0 {
				t.Errorf("%s with %s = %q status %d, want %q at 0",
					c.src, side.name, strings.TrimSpace(out), st, side.want)
			}
		}
	}
}

// The plain value of a whole subscript on a scalar is *not* this question.
// `${h[@]}` is the one value the name holds under either reading, and a row
// that moved with the axis would mean the decline had been written on the
// subscript rather than on the slice.
func TestAWholeSubscriptOnAScalarIsTheValueUnderEitherReading(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		out, st := sliceOnAScalarRun(t, `h="a b"; set -- "${h[@]}"; echo "$# [$1]"`, answer)
		if strings.TrimSpace(out) != "1 [a b]" || st != 0 {
			t.Errorf("with %v = %q status %d, want %q at 0", answer, strings.TrimSpace(out), st, "1 [a b]")
		}
	}
}

// The length is a different question, and pinning it here is what keeps the
// two from being answered by one field again: a shell can count a list of one
// for `${#h[@]}` and still slice the value's characters, which is what four of
// the five shells with the construct do.
func TestTheLengthAndTheSliceOfAWholeSubscriptOnAScalarAreSeparateQuestions(t *testing.T) {
	run := func(measures, slices Answer) (string, int) {
		return runGrammar(t, `h="a b"; echo "${#h[@]} ${h[@]:0:1}"`, func(d *syntax.Dialect) {
			d.ArraySubscript = true
			d.ParamSubstring = true
		}, func(r *Runner) {
			sem := CoreSemantics()
			sem.ArrayBaseIsZero = Yes
			sem.ArrayScalarIsTheWholeArray = No
			sem.SplitParamExpansion = No
			sem.WholeSubscriptOnAScalarMeasuresIt = measures
			sem.WholeSubscriptOnAScalarSlicesIt = slices
			r.Semantics = &sem
		})
	}
	for _, c := range []struct {
		measures, slices Answer
		want             string
	}{
		{No, Yes, "1 a"},
		{Yes, Yes, "3 a"},
		{No, No, "1 a b"},
		{Yes, No, "3 a b"},
	} {
		out, st := run(c.measures, c.slices)
		if strings.TrimSpace(out) != c.want || st != 0 {
			t.Errorf("measures=%v slices=%v = %q status %d, want %q at 0",
				c.measures, c.slices, strings.TrimSpace(out), st, c.want)
		}
	}
}
