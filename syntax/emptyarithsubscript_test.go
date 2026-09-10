// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A subscript written with nothing between the brackets parses, and the node
// says so: Index is nil and Sub is empty, neither of which alone could — a
// name with no subscript at all is an ArithVar and never reaches here.
//
// The grammar carries it rather than refusing it because every shell in the
// panel that has array subscripts parses it and answers at run time, and the
// three run-time answers are a semantics axis. It is reached far more often
// than it is written: an arithmetic expansion substitutes its parameters
// before it parses, so `$(( a[$w] ))` with an empty `$w` *is* `$(( a[] ))` by
// the time the expression exists (#1745).
func TestAnEmptyArithmeticSubscriptParses(t *testing.T) {
	d := Core()
	x, ok := parseArithOf(t, `a[]`, d).(*ArithIndex)
	if !ok {
		t.Fatalf("a[] parsed as %T, want *ArithIndex", parseArithOf(t, `a[]`, d))
	}
	if x.Name != "a" || x.Index != nil || x.Sub != "" || !x.Empty {
		t.Errorf("a[] = %+v, want the name with an empty subscript", x)
	}
	// It is an operand like any other, so it composes.
	if got := parseArithOf(t, `1 + a[]`, d); got == nil {
		t.Errorf("1 + a[] did not parse")
	}
	// A subscript that holds anything at all is not this: the empty flag is
	// off and the expression is there, which is what keeps the run-time
	// answer for `a[]` from reaching `a[0]`.
	if y, ok := parseArithOf(t, `a[0]`, d).(*ArithIndex); !ok || y.Empty || y.Index == nil {
		t.Errorf("a[0] = %+v, want a subscript that is not empty", y)
	}
}

// It rides on the flag that admits subscripts at all: where a dialect has
// none, `a[]` is a name followed by text that cannot be an operator, and the
// expression is not built.
//
// A nil expression is what a `$(( ))` that would not parse leaves behind —
// the failure is carried to run time, because the text an expression is read
// from is not final until its parameters have gone in. So nil here is "not
// accepted", which is the same answer `a[0]` gets without the flag and the
// answer `a[]` got in every dialect before this.
func TestAnEmptyArithmeticSubscriptNeedsTheFlag(t *testing.T) {
	for _, src := range []string{`a[]`, `a[0]`} {
		if got := parseArithOf(t, src, POSIX()); got != nil {
			t.Errorf("%s parsed as %T without ArraySubscript, want nothing", src, got)
		}
	}
}
