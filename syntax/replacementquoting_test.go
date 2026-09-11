// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The parser keeps both readings of a replacement operand, and keeps the
// second one only where the two could come to different text — which is what
// makes its presence the question rather than an alternative answer (#1209).
func replacementOf(t *testing.T, src string) *ParamExpr {
	t.Helper()
	f, err := Parse(src, Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	for _, w := range sc.Args {
		for _, s := range w.Spans {
			if s.Kind == ParamExp && s.Param != nil && s.Param.Op == ParamReplace {
				return s.Param
			}
		}
	}
	t.Fatalf("no replacement expansion in %q", src)
	return nil
}

func TestASecondReadingIsKeptOnlyWhereItCouldDiffer(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		kept bool
	}{
		{"a single quote, quoted", `printf "%s" "${s/a/'q'}"`, true},
		{"a backslash, quoted", `printf "%s" "${s/a/\q}"`, true},
		{"a leading tilde, quoted", `printf "%s" "${s/a/~}"`, true},

		// Both readings answer these alike, so there is nothing to ask.
		{"a plain word", `printf "%s" "${s/a/b}"`, false},
		{"a parameter", `printf "%s" "${s/a/$v}"`, false},
		{"a double quote, which both readings remove", `printf "%s" "${s/a/"q"}"`, false},
		{"a tilde that is not at the front", `printf "%s" "${s/a/p~q}"`, false},
		{"a glob character", `printf "%s" "${s/a/*}"`, false},

		// Unquoted there is no enclosing quoting to take, which is where the
		// whole panel agrees.
		{"a single quote, unquoted", `printf "%s" ${s/a/'q'}`, false},

		// And the *pattern* half never takes it: its quotes quote in every
		// column, so a second reading there would be a question nobody asked.
		{"a quote in the pattern half", `printf "%s" "${s/'a'/Z}"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := replacementOf(t, tc.src)
			if got := e.Arg2Enclosed != nil; got != tc.kept {
				t.Errorf("%s: Arg2Enclosed kept = %v, want %v", tc.src, got, tc.kept)
			}
			if e.Arg2 == nil {
				t.Errorf("%s: Arg2 is nil; the word reading is always built", tc.src)
			}
		})
	}
}

// The two readings are different trees, which is why both are built rather
// than one being derived from the other at run time: under the word reading
// the quotes quote and what stands between them is one literal, and under the
// enclosing one they are characters with a live expansion between them.
func TestTheTwoReadingsAreDifferentTrees(t *testing.T) {
	e := replacementOf(t, `printf "%s" "${s/a/'$v'}"`)
	if n := len(e.Arg2.Spans); n != 1 || e.Arg2.Spans[0].Kind != Literal {
		t.Errorf("the word reading is %d spans, want one literal", n)
	}
	if e.Arg2Enclosed == nil {
		t.Fatalf("no second reading was kept")
	}
	var sawParam bool
	for _, s := range e.Arg2Enclosed.Spans {
		if s.Kind == ParamExp {
			sawParam = true
		}
	}
	if !sawParam {
		t.Errorf("the enclosing reading is %+v, want the parameter still live", e.Arg2Enclosed.Spans)
	}
}
