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

// An element assignment the shell *refuses* gives up as far as the dialect
// gives up for any other failed expansion — the line in one column and the
// whole file in the other three.
//
// It ended the file in every column. Semantics.FailedExpansionAbandonsTheLine
// already names "a bad subscript" among the failures it governs and its own
// suite records that the subscript row is asserted "in the dialect that has
// arrays"; no such row existed, so the four sites that refuse a subscript
// went on calling Runner.fatal and one unevaluable subscript cost a whole
// script. That is what left a file of bash's own suite ending at 1 where bash
// ends at 0 (#2299).
//
// Measured 2026-09-16 from a script file, the refusal on one line and
// `echo "NEXT=$?"` on the next — which is the pairing the question has to be
// asked on, since on one line giving up the list and giving up the file print
// the same nothing:
//
//	                            bash 5.3  bash 3.2  zsh 5.9  ksh93
//	a[1+]=q                     NEXT=1    NEXT=1    stops    stops
//	a[1+]+=q                    NEXT=1    NEXT=1    stops    stops
//	v="1+"; a[$v]=q             NEXT=1    NEXT=1    stops    stops
//	a=(1 2 3); a[-9]=q          NEXT=1    NEXT=1    —        —
//	a=([1+]=x)                  NEXT=1    NEXT=1    stops    —
//	a=(1 2 3); a[1]=(q)         NEXT=1    NEXT=1    —        —
//
// An em dash is a column that does not refuse there at all and so never
// arrives here.
func TestARefusedElementAssignmentGivesUpTheLineOrTheFile(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		// Each of the four sites that raise the refusal, because they are
		// four functions and a fix reaching one of them passes a single row.
		{"a written subscript that will not evaluate", "a[1+]=q"},
		{"the same subscript appended to", "a[1+]+=q"},
		{"a subscript arriving from an expansion", `v="1+"; a[$v]=q`},
		{"a subscript below the first element", "a=(1 2 3); a[-9]=q"},
		{"a literal's own subscript", "a=([1+]=x)"},
		{"a literal standing where one element's value goes", "a=(1 2 3); a[1]=(q)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src + "\necho \"NEXT=$?\"\n"

			out, errs, st := subscriptAbandonRun(t, src, Yes)
			if errs == "" {
				t.Fatalf("giving up the line: nothing was reported, so this row is not the refusal it names")
			}
			if !strings.Contains(out, "NEXT=") {
				t.Fatalf("giving up the line: output %q, want the next line to run", out)
			}
			if !strings.Contains(out, "NEXT=1") {
				t.Errorf("giving up the line: output %q, want the refusal's 1 left behind", out)
			}
			if st != 0 {
				t.Errorf("giving up the line: status %d, want the next line's 0", st)
			}

			out, errs, st = subscriptAbandonRun(t, src, No)
			if errs == "" {
				t.Fatalf("ending the file: nothing was reported, so the refusal was not reached")
			}
			if strings.Contains(out, "NEXT=") {
				t.Errorf("ending the file: output %q, want nothing after it", out)
			}
			if st == 0 {
				t.Errorf("ending the file: status %d, want the refusal's", st)
			}
		})
	}
}

// The control that says the unit is the line: with a `;` the rest of the list
// goes under both answers, so a probe of that shape cannot tell the two
// readings apart. Every earlier probe of this path was that shape, which is
// how the whole family stayed invisible.
func TestARefusedElementAssignmentGivesUpItsListUnderEitherAnswer(t *testing.T) {
	for _, abandons := range []Answer{Yes, No} {
		out, _, _ := subscriptAbandonRun(t, "a[1+]=q; echo SAME\necho NEXT\n", abandons)
		if strings.Contains(out, "SAME") {
			t.Errorf("abandons=%v: output %q, want the rest of the list given up", abandons, out)
		}
	}
}

// And the give-up unwinds past every shape that holds statements, which is
// controlAbandon's definition rather than an extra of this path. A construct
// that swallowed it would carry on inside itself and complain a second time.
func TestARefusedElementAssignmentUnwindsPastEveryEnclosingShape(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a loop", "for i in 1 2 3; do a[1+]=q; echo ONE; done\necho TWO"},
		{"an if", "if true; then a[1+]=q; echo ONE; fi\necho TWO"},
		{"a group", "{ a[1+]=q; echo ONE; }\necho TWO"},
		{"a function body", "f() { a[1+]=q; echo ONE; }\nf\necho TWO"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := subscriptAbandonRun(t, tc.src+"\n", Yes)
			if n := strings.Count(errs, "\n"); n != 1 {
				t.Errorf("complained %d times, want once: %q", n, errs)
			}
			if strings.Contains(out, "ONE") {
				t.Errorf("output %q, want what enclosed it given up", out)
			}
			if !strings.Contains(out, "TWO") {
				t.Errorf("output %q, want the shell to carry on", out)
			}
			if st != 0 {
				t.Errorf("status %d, want the last command's", st)
			}
		})
	}
}

// An element assignment that *works* still reports success, which is what
// says the new door is opened by a refusal and not by a subscript.
func TestAnElementAssignmentThatWorkedStillReportsSuccess(t *testing.T) {
	for _, src := range []string{
		"a=(1 2 3); a[1]=q\necho \"NEXT=$?\"\n",
		"a=(1 2 3); a[1+1]=q\necho \"NEXT=$?\"\n",
		"a=([2]=c)\necho \"NEXT=$?\"\n",
	} {
		out, errs, st := subscriptAbandonRun(t, src, Yes)
		if errs != "" {
			t.Errorf("%q said %q, want nothing", src, errs)
		}
		if !strings.Contains(out, "NEXT=0") {
			t.Errorf("%q said %q, want NEXT=0", src, out)
		}
		if st != 0 {
			t.Errorf("%q: status %d, want 0", src, st)
		}
	}
}

func subscriptAbandonRun(t *testing.T, src string, abandons Answer) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := testSemantics()
	sem.FailedExpansionAbandonsTheLine = abandons
	sem.FatalErrorStatusIsOne = Yes
	// The refusal one of the rows is about, which is a policy rather than a
	// yes-or-no: the core leaves it unanswered and an unanswered axis reports
	// itself and stops, which is not the refusal this suite measures.
	sem.SubscriptedArrayLiteral = SubscriptedArrayLiteralRefused
	var out, errs strings.Builder
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}
