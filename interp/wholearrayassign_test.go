// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The whole-array spelling on the **left** of an assignment — `a[@]=Z` and
// `a[*]=Z` — which the panel answers five ways over two questions. See
// Semantics.WholeArraySubscriptAssigningAnArray and #2285.
//
// Tests name axes and wordings, never shells.

func wholeAssign(t *testing.T, src string, array, table WholeArraySubscriptAssignPolicy) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
		d.DeclarationUtilities = map[string]bool{"typeset": true}
	}, func(r *Runner) {
		s := *r.Semantics
		s.WholeArraySubscriptAssigningAnArray = array
		s.WholeArraySubscriptAssigningATable = table
		r.Semantics = &s
		d := CoreDiagnostics()
		if r.Diagnostics != nil {
			d = *r.Diagnostics
		}
		d.BadArraySubscript = "%[1]s[%[2]s]: bad array subscript"
		d.InvalidSubscriptInAssignment = "%s: invalid subscript in assignment"
		d.SliceOfAnAssociativeArray = "%[1]s: attempt to set slice of associative array"
		r.Diagnostics = &d
	})
}

// The taking answer replaces every element with the one value, in both
// spellings — the headline, and the row that says it is not "write element
// zero and leave the rest".
func TestAWholeArraySubscriptNamesEveryElement(t *testing.T) {
	for _, sub := range []string{"@", "*"} {
		src := `x=(p q); x[` + sub + `]=Z; echo "n=${#x[@]} [${x[*]}]"`
		out, st := wholeAssign(t, src, WholeArraySubscriptNamesEveryElement, WholeArraySubscriptIsASliceOfATable)
		if got := strings.TrimSpace(out); got != "n=1 [Z]" || st != 0 {
			t.Errorf("%s = %q (status %d), want one element", src, got, st)
		}
	}
}

// It replaces the *name* and not the elements it finds, which is what a
// scalar and an unset name say: both come out a one-element array. A
// splice-into-what-is-there reading would leave the scalar a scalar.
func TestTheWholeArraySubscriptWritesOverAnyKindOfName(t *testing.T) {
	for _, src := range []string{
		`v=s; v[@]=Z; echo "n=${#v[@]} [${v[*]}]"`,
		`u[@]=Z; echo "n=${#u[@]} [${u[*]}]"`,
	} {
		out, st := wholeAssign(t, src, WholeArraySubscriptNamesEveryElement, WholeArraySubscriptIsASliceOfATable)
		if got := strings.TrimSpace(out); got != "n=1 [Z]" || st != 0 {
			t.Errorf("%s = %q (status %d), want one element", src, got, st)
		}
	}
}

// The append adds one element at the end rather than joining every element.
func TestAWholeArraySubscriptAppendAddsOneElement(t *testing.T) {
	out, st := wholeAssign(t, `x=(p q); x[@]+=Z; echo "n=${#x[@]} [${x[*]}]"`,
		WholeArraySubscriptNamesEveryElement, WholeArraySubscriptIsASliceOfATable)
	if got := strings.TrimSpace(out); got != "n=3 [p q Z]" || st != 0 {
		t.Errorf("= %q (status %d), want the value added at the end", got, st)
	}
}

// The bad-subscript answer: reported, 1, the name untouched, and the rest of
// the *command list* given up — with the next line still running, which is
// the half a one-line probe cannot see.
func TestTheWholeArraySubscriptRefusedAsABadSubscript(t *testing.T) {
	const src = "x=(p q)\nx[@]=Z; echo same-line\necho \"n=${#x[@]}\"; echo next-line"
	out, _ := wholeAssign(t, src, WholeArraySubscriptIsABadSubscript, WholeArraySubscriptIsAnOrdinaryKey)
	if !strings.Contains(out, "x[@]: bad array subscript") {
		t.Errorf("= %q, want the bad-subscript refusal naming the name and the subscript", out)
	}
	if strings.Contains(out, "same-line") {
		t.Errorf("= %q, want the rest of the list given up", out)
	}
	for _, want := range []string{"n=2", "next-line"} {
		if !strings.Contains(out, want) {
			t.Errorf("= %q, want %q — the array untouched and the next line run", out, want)
		}
	}
}

// The invalid-subscript answer names the *subscript* rather than the name,
// and ends the input rather than the line.
func TestTheWholeArraySubscriptRefusedAsInvalid(t *testing.T) {
	const src = "x=(p q)\nx[@]=Z\necho next-line"
	out, st := wholeAssign(t, src, WholeArraySubscriptIsInvalidInAnAssignment, WholeArraySubscriptIsInvalidInAnAssignment)
	if !strings.Contains(out, "@: invalid subscript in assignment") {
		t.Errorf("= %q, want the refusal naming the subscript", out)
	}
	if strings.Contains(out, "next-line") || st == 0 {
		t.Errorf("= %q (status %d), want the input to end", out, st)
	}
}

// The two fields are separate, and this is the row that says so: the same
// spelling over a **table** reaches the other one, and one answer may take
// where the other refuses.
func TestTheTableFieldIsAskedForATable(t *testing.T) {
	const src = `typeset -A m; m[k]=v; m[@]=Z; echo "st=$?"; echo "[${m[@]}]"`

	// The taking answer for a table is an ordinary key, and it does not
	// disturb the key that was there.
	out, st := wholeAssign(t, src, WholeArraySubscriptIsABadSubscript, WholeArraySubscriptIsAnOrdinaryKey)
	if st != 0 || !strings.Contains(out, "st=0") || strings.Contains(out, "bad array subscript") {
		t.Errorf("key: = %q (status %d), want the key stored and the array field unasked", out, st)
	}

	// And the refusing answer for a table names the name and ends the input,
	// with the array field taking — so neither field can be standing in for
	// the other.
	out, _ = wholeAssign(t, src, WholeArraySubscriptNamesEveryElement, WholeArraySubscriptIsASliceOfATable)
	if !strings.Contains(out, "m: attempt to set slice of associative array") {
		t.Errorf("slice: = %q, want the table refusal", out)
	}
	if strings.Contains(out, "st=") {
		t.Errorf("slice: = %q, want the input to end", out)
	}
}

// Only the two characters as they were **typed**. A subscript that merely
// expands to `@`, or one that was quoted, is an arithmetic subscript like any
// other and must reach neither field.
func TestOnlyABareWholeArraySubscriptIsOne(t *testing.T) {
	for _, src := range []string{
		`x=(p q); i=@; x[$i]=Z; echo "n=${#x[@]}"`,
		`x=(p q); x["@"]=Z; echo "n=${#x[@]}"`,
	} {
		out, _ := wholeAssign(t, src, WholeArraySubscriptAssignUnspecified, WholeArraySubscriptAssignUnspecified)
		if strings.Contains(out, "disagree here") {
			t.Errorf("%s = %q, want no question at all", src, out)
		}
	}
}

// An ordinary subscript asks nothing either, which is what keeps the fields
// off every element write in every script.
func TestAnOrdinarySubscriptAsksNeitherField(t *testing.T) {
	out, st := wholeAssign(t, `x=(p q); x[0]=Z; echo "[${x[*]}]"`,
		WholeArraySubscriptAssignUnspecified, WholeArraySubscriptAssignUnspecified)
	if st != 0 || strings.Contains(out, "disagree here") {
		t.Errorf("= %q (status %d), want the ordinary element write", out, st)
	}
}

// An unanswered field refuses rather than picking a shell, and says which of
// the two was asked.
func TestAnUnansweredWholeArraySubscriptRefuses(t *testing.T) {
	out, st := wholeAssign(t, `x=(p q); x[@]=Z`, WholeArraySubscriptAssignUnspecified, WholeArraySubscriptAssignUnspecified)
	if st == 0 || !strings.Contains(out, "whole-array subscript on the left of an assignment") {
		t.Errorf("= %q (status %d), want the unanswered refusal", out, st)
	}
	out, st = wholeAssign(t, `typeset -A m; m[@]=Z`, WholeArraySubscriptNamesEveryElement, WholeArraySubscriptAssignUnspecified)
	if st == 0 || !strings.Contains(out, "assignment to a table") {
		t.Errorf("table: = %q (status %d), want the table field's unanswered refusal", out, st)
	}
}
