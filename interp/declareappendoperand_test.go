// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration builtin's operand may carry the append operator, in the one
// shell whose declarations take it: `declare a=1; declare a+=2` is `12`.
//
// It was refused as a bad name — `a+` is not an identifier — which is the
// right answer in three of the four and the wrong one in the fourth, and the
// refusal was not worded the way any of the three word it either (#1503).
//
// See Semantics.DeclarationTakesAnAppendOperand.
func TestADeclarationTakesAnAppendOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", `typeset a=1; typeset a+=2; echo "[$a]"`, "[12]\n"},
		{"a name with nothing in it", `unset a; typeset a+=2; echo "[$a]"`, "[2]\n"},
		{"an empty value joins nothing", `a=1; typeset a+=; echo "[$a]"`, "[1]\n"},
		// The attribute decides which join this is, which is what makes this
		// the statement's own append rather than a string concatenation.
		{"an integer name adds", `typeset -i a=1; typeset a+=2; echo "[$a]"`, "[3]\n"},
		// What the name is holding decides which store the value reaches.
		{"an array joins its first element", `arr=(p q); typeset arr+=x; echo "[${arr[*]}]"`, "[px q]\n"},
		// Two operands on one line, so the reading is per operand.
		{"beside an ordinary operand", `typeset a=1 a+=2 b=9; echo "[$a][$b]"`, "[12][9]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src, withAppendOperand(Yes), Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q at 0", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// Every declaration builtin takes it, which is four loops over one operand
// rather than one — the shape a declaration bug here has taken before, where
// only the loop somebody edited got the rule.
func TestEveryDeclarationBuiltinTakesTheAppendOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"typeset", `a=1; typeset a+=2; echo "[$a]"`, "[12]\n"},
		{"export", `a=1; export a+=2; echo "[$a]"`, "[12]\n"},
		{"readonly", `a=1; readonly a+=2; echo "[$a]"`, "[12]\n"},
		{"local", `f() { local a=1; local a+=2; echo "[$a]"; }; f`, "[12]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src, withAppendOperand(Yes), Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q at 0", tc.src, out, errs, st, tc.want)
			}
		})
	}
	// `export` and `readonly` take no scope, so the join inside a function is
	// over the value the name already reads — the other side of the fresh
	// cell the declarations that do take one write into.
	for _, tc := range []struct{ name, src, want string }{
		{"export in a call", `a=1; f() { export a+=2; }; f; echo "[$a]"`, "[12]\n"},
		{"readonly in a call", `a=1; f() { readonly a+=2; echo "[$a]"; }; f`, "[12]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src, withAppendOperand(Yes), Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q at 0", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// The value joins the cell the declaration writes, which is the same cell a
// value without the operator would land in — so a declaration that takes a
// fresh scope has nothing to join to, whatever the name outside it holds.
func TestAnAppendOperandJoinsTheCellTheDeclarationWrites(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a fresh local joins nothing",
			`a=1; f() { typeset a+=2; echo "in [$a]"; }; f; echo "out [$a]"`,
			"in [2]\nout [1]\n",
		},
		{
			"a second declaration in the same scope joins the first",
			`f() { typeset a=1; typeset a+=2; echo "in [$a]"; }; f`,
			"in [12]\n",
		},
		{
			"a declaration that takes no scope joins what is there",
			`a=1; f() { typeset -g a+=2; echo "in [$a]"; }; f; echo "out [$a]"`,
			"in [12]\nout [12]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src, withAppendOperand(Yes), Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q at 0", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// The other answer: the `+` is part of a name that is not one, and the
// complaint names `a+` rather than the whole operand — which is what the
// three shells that refuse it say, two of them otherwise quoting a bad
// operand back in full.
func TestADeclarationRefusingTheAppendOperandNamesTheNameAlone(t *testing.T) {
	dg := Diagnostics{BuiltinBadName: map[string]string{"typeset": "%[1]s: %[2]s: not a name"}}
	out, errs, st := declRun(t, `a=1; typeset a+=2; echo "[$a]"`, withAppendOperand(No), dg)
	_ = st
	if want := "testsh: typeset: a+: not a name\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if out != "[1]\n" {
		t.Errorf("= %q, want the value untouched", out)
	}
	// And the whole operand where the name is not one for a reason of its
	// own, which is the row that says the `+` is what is being read here and
	// not simply the text in front of the `=`.
	dg2 := Diagnostics{
		BuiltinBadName:           map[string]string{"typeset": "%[1]s: %[2]s: not a name"},
		BuiltinBadNameKeepsValue: true,
	}
	_, errs, _ = declRun(t, `typeset 1x=2`, withAppendOperand(No), dg2)
	if want := "testsh: typeset: 1x=2: not a name\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
}

// Asked only for the one shape that splits the panel. A `+` with no `=` after
// it is not this spelling — it is refused as a name in every column — so a
// dialect that never answered the axis is never asked about it.
func TestAPlusWithNoValueIsNotTheAppendOperand(t *testing.T) {
	dg := Diagnostics{BuiltinBadName: map[string]string{"typeset": "%[1]s: %[2]s: not a name"}}
	for _, answer := range []Answer{Yes, No, Unspecified} {
		out, errs, st := declRun(t, `a=1; typeset a+; echo "[$a]"`, withAppendOperand(answer), dg)
		if want := "testsh: typeset: a+: not a name\n"; errs != want {
			t.Errorf("answer %v: stderr = %q, want %q", answer, errs, want)
		}
		if out != "[1]\n" {
			t.Errorf("answer %v: = %q (status %d), want the value untouched", answer, out, st)
		}
	}
}

// And where the shape is there and no dialect answered, it is refused by name
// rather than one of the two answers being picked.
func TestAnUnansweredAppendOperandAxisIsRefused(t *testing.T) {
	out, errs, st := declRun(t, `a=1; typeset a+=2`, withAppendOperand(Unspecified), Diagnostics{})
	want := "testsh: a declaration taking a `name+=value` operand: " +
		"the shells disagree here and no dialect was chosen\n"
	if errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if out != "" || st != 2 {
		t.Errorf("= %q (status %d), want nothing declared at 2", out, st)
	}
	// And the name is untouched, which is the half a status alone does not
	// say: an axis reported and then answered anyway is the silent wrong
	// answer this refusal exists to avoid.
	out, _, _ = declRun(t, `a=1; typeset a+=2; echo "[$a]"`, withAppendOperand(Unspecified), Diagnostics{})
	if !strings.HasSuffix(out, "[1]\n") {
		t.Errorf("= %q, want the value untouched", out)
	}
}

// withAppendOperand answers the axis under test, and the neighbours these
// rows walk past: a local declared by the word rather than by the keyword, a
// scalar appended to an array joining its first element rather than becoming
// one, a bad name reported rather than ending the script, and the `-g` letter
// that says which cell a declaration writes.
func withAppendOperand(answer Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclarationTakesAnAppendOperand = answer
		s.TypesetLocalNeedsKeywordFunction = No
		s.ScalarAppendedToAnArrayBecomesANewElement = No
		s.BadNameToDeclarationFatal = No
		s.DeclareOptions = "aAgilnprtuxF"
	}
}
