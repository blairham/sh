// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A failure raised while evaluating a subscript is blamed on the subscript's
// own text, wherever the brackets stand and whatever the name in front of
// them is (#2420).
//
// Measured 2026-09-14 on ksh93u+ 2012-08-01 and bash 5.3.15: `$(( nodecl[1/0]
// ))`, `$(( 2 + nodecl[1/0] ))`, `$(( x[1/0] ))` and `$(( a[1/0] ))` over a
// declared array all draw the one sentence, `1/0: divide by zero` in ksh93
// and `1/0: division by 0 (error token is "0")` in bash. Ours quoted the
// whole expression, which named `nodecl[1/0]` where both shells name the
// three characters that failed.
//
// The wordings here are the substrate's own shape — the expression, a colon
// and the reason — so the assertion is about the *extent* and not about which
// shell writes which sentence.
func TestASubscriptFailureIsBlamedOnTheSubscript(t *testing.T) {
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"a name nothing declared",
			`echo $(( nodecl[1/0] ))`, "1/0: division by zero",
			"the row the issue was filed on: the name in front of the brackets is not part of what failed",
		},
		{
			"the brackets inside a larger expression",
			`echo $(( 2 + nodecl[1/0] ))`, "1/0: division by zero",
			"the extent is the brackets' contents and not the expression that holds them",
		},
		{
			"a plain name",
			`echo $(( x[1/0] ))`, "1/0: division by zero",
			"a shorter name, so a reading that sliced the subscript out of the name by search would land elsewhere",
		},
		{
			"a declared array",
			`a=(1 2 3); echo $(( a[1/0] ))`, "1/0: division by zero",
			"the same answer where the subscript really is an index into something",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := runForArithBlame(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q — %s", out, tc.want, tc.why)
			}
			if strings.Contains(out, "nodecl[") || strings.Contains(out, "x[") || strings.Contains(out, "a[") {
				t.Errorf("got %q, which still quotes the name and the brackets — %s", out, tc.why)
			}
		})
	}
}

// The control the rule above needs: an expression that is not a subscript is
// still blamed as a whole, so nothing here narrows a complaint that was right.
func TestAnExpressionOutsideBracketsIsStillBlamedWhole(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The blanks are kept, which is what says the text is the
		// expression as written and not a trimmed copy of it — the
		// subscript's own text carries none because the brackets hold none.
		{"a division at the top level", `echo $(( 1/0 ))`, " 1/0 : division by zero"},
		{"a division inside a larger expression", `echo $(( 2 + 1/0 ))`, " 2 + 1/0 : division by zero"},
		{"an empty pair of brackets has nothing to quote", `echo $(( a[] ))`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := runForArithBlame(t, tc.src)
			if tc.want == "" {
				return
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

func runForArithBlame(t *testing.T, src string) string {
	t.Helper()
	d := syntax.Core()
	d.ArraySubscript = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := PosixSemantics()
	sem.ArithSubscriptSkippedWhenNameUnset = No
	dg := Diagnostics{ArithError: "%[1]s: %[2]s"}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Dialect: &d, Semantics: &sem, Diagnostics: &dg,
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String()
}
