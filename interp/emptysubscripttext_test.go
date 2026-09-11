// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subscript whose *text* came out empty or blank once it was expanded —
// `${a[$w]}` with an empty `$w` — which one column will not read as an
// expression at all. The written `${a[]}` beside it is a different axis and
// a different sentence; see emptyparamsubscript_test.go.

// emptySubText answers the axis, and the wording the refusing answer needs.
func emptySubText(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := CoreSemantics()
		if r.Semantics != nil {
			s = *r.Semantics
		}
		s.EmptySubscriptTextIsAMathError = a
		r.Semantics = &s
		d := CoreDiagnostics()
		if r.Diagnostics != nil {
			d = *r.Diagnostics
		}
		d.EmptySubscriptTextExpanded = "the subscript expanded to nothing"
		r.Diagnostics = &d
	}
}

func TestASubscriptThatExpandedToNothingAnswersByAxis(t *testing.T) {
	const setup = `a=(5 6 7); s=hi; w=; `
	for _, tc := range []struct{ name, src, read string }{
		{"an indexed array", `echo "[${a[$w]}]"`, "[5]"},
		{"a scalar", `echo "[${s[$w]}]"`, "[hi]"},
		{"a name nothing declared", `echo "[${nodecl[$w]}]"`, "[]"},
		{"under a length", `echo "[${#a[$w]}]"`, "[1]"},
		{"an element assignment", `a[$w]=z; echo "[${a[0]}]"`, "[z]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, setup+tc.src+`; echo after`, subscripting, emptySubText(No))
			if want := tc.read + "\nafter\n"; out != want || st != 0 {
				t.Errorf("read: %s = %q (status %d), want %q at 0", tc.src, out, st, want)
			}
			out, st = runGrammar(t, setup+tc.src+`; echo after`, subscripting, emptySubText(Yes))
			if !strings.Contains(out, "the subscript expanded to nothing") {
				t.Errorf("refused: %s = %q, want the dialect's sentence", tc.src, out)
			}
			if strings.Contains(out, "after") || st == 0 {
				t.Errorf("refused: %s = %q (status %d), want the input to end", tc.src, out, st)
			}
		})
	}
}

// Empty and blank are two sentences rather than one, because the shell that
// refuses them gives two: nothing at all is its own complaint, and blanks are
// the expression running out — which is the sentence `$(( a[ ] ))` already
// earns. The blank one is not the dialect's EmptySubscriptTextExpanded, so a
// test that looked only for that wording would pass on a reading that gave
// the same answer to both.
func TestABlankSubscriptIsTheExpressionRunningOut(t *testing.T) {
	for _, src := range []string{
		`a=(5 6 7); echo "[${a[ ]}]"`,
		`a=(5 6 7); w="  "; echo "[${a[$w]}]"`,
	} {
		out, st := runGrammar(t, src+`; echo after`, subscripting, emptySubText(Yes))
		if strings.Contains(out, "the subscript expanded to nothing") {
			t.Errorf("%s = %q, want the expression to run out rather than the empty sentence", src, out)
		}
		if strings.Contains(out, "after") || st == 0 {
			t.Errorf("%s = %q (status %d), want the input to end", src, out, st)
		}
	}
}

// What the axis does not reach, and each row is a place the same emptiness
// means something else.
func TestTheEmptySubscriptAxisIsNotAskedElsewhere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// A key is not an expression, so the empty one is the empty key.
		{"an association's subscript", `typeset -A m; m[k]=v; w=; echo "[${m[$w]}]"`, "[]\n"},
		// A substring's offset that expanded to nothing is zero in every
		// column, the refusing one included.
		{"a substring's offset", `x=abcdef; w=; echo "[${x:$w:2}]"`, "[ab]\n"},
		// And a subscript with something in it is read as it always was.
		{"a subscript with text in it", `a=(5 6 7); echo "[${a[ 2 ]}]"`, "[7]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []Answer{Yes, No} {
				out, st := runGrammar(t, tc.src, subscripting, emptySubText(a))
				if out != tc.want || st != 0 {
					t.Errorf("axis %v: %s = %q (status %d), want %q at 0", a, tc.src, out, st, tc.want)
				}
			}
		})
	}
}
