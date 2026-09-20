// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `-p` listing whose own operand carries a value. Three readings, and the
// tests name the axis (DeclarePrintPerformsItsOperand) rather than a shell —
// see interp/declareprintoperand.go for the rows each was measured from.

// bare is the listing form these tests read, so that a row is the assignment
// a script would have written rather than a command word and a flag cluster.
func printOperand(v DeclarePrintOperandPolicy) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclareListing = DeclareListingBareAssignments
		s.DeclareValueQuoting = ListingQuoteWhenNeededDollar
		s.DeclarePrintReportsAMissingName = No
		s.DeclarePrintPerformsItsOperand = v
	}
}

// TestDeclarePrintPerformsItsOperandPlainly: the operand is stored and the
// listing shows what was stored, and the store carries none of the line's
// letters.
func TestDeclarePrintPerformsItsOperandPlainly(t *testing.T) {
	plain := printOperand(DeclarePrintOperandIsAssignedPlainly)
	for _, tc := range []struct{ src, want string }{
		// The listing is of what this very command stored.
		{`typeset -p s=5; echo "[$s]"`, "s=5\n[5]"},
		// An array literal operand lands through the command's own operand
		// assignment, which runs after the builtin — so this row is the one
		// that says the listing waited for it.
		{`typeset -p e=(1 2); echo "[${e[@]}]"`, "typeset -a e=(1 2)\n[1 2]"},
		// A subscripted operand writes the element and the listing is of the
		// variable it reached, not of the brackets.
		{`typeset -p q[2]=7; echo "[${q[2]}]"`, "typeset -a q=([2]=7)\n[7]"},
		// Every store runs before any listing: two operands writing one name
		// both show the second value. A listing per operand would write 1
		// and then 2.
		{`typeset -p c=1 c=2`, "c=2\nc=2"},
		// The line's letters do not land: the value is the text and not the
		// sum it would be under the integer attribute.
		{`typeset -ip n=3+3; echo "[$n]"`, "n=3+3\n[3+3]"},
		// Nor is a scope taken, so the name outlives the function.
		{`f() { typeset -p l=7; }; f; echo "[$l]"`, "l=7\n[7]"},
		// An appended operand is performed and contributes no row: the text
		// before the `=` is not a name this shell has.
		{`s=ab; typeset -p s+=cd; echo "[$s]"`, "[abcd]"},
		// A word that is no name at all stores nothing and lists nothing.
		{`typeset -p 'bad name=1'; echo "[${v-unset}]"`, "[unset]"},
		// The control: an operand carrying no value is the ordinary listing,
		// which every reading of the axis answers the same way.
		{`v=1; typeset -p v`, "v=1"},
	} {
		out, errs, st := declareRun(t, tc.src, plain, Diagnostics{})
		if strings.TrimSuffix(out, "\n") != tc.want || st != 0 || errs != "" {
			t.Errorf("%s = %q (stderr %q, status %d), want %q",
				tc.src, out, errs, st, tc.want)
		}
	}
}

// TestDeclarePrintPlainOperandIsRefusedAsABareAssignment: the store is a
// bare assignment's and not a declaration's, which is what the refusal a
// frozen name earns says — the sentence carries no builtin name, where
// `typeset rr=2` on the same name carries one.
func TestDeclarePrintPlainOperandIsRefusedAsABareAssignment(t *testing.T) {
	plain := printOperand(DeclarePrintOperandIsAssignedPlainly)
	dg := Diagnostics{
		ReadonlyVariable:              "%[1]s: is read only",
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
		ReadonlyRefusalNamesBuiltin:   map[string]bool{"typeset": true},
	}
	_, errs, _ := declareRun(t, `typeset -r rr=1; typeset -p rr=2`, plain, dg)
	if want := "rr: is read only"; !strings.Contains(errs, want) {
		t.Errorf("stderr %q, want %q with no builtin in front of it", errs, want)
	}
	if strings.Contains(errs, "typeset: rr:") {
		t.Errorf("stderr %q, want the bare assignment's wording", errs)
	}
}

// TestDeclarePrintDeclaresALiteralOperand: only the operand the parser kept
// apart as an array literal is performed, and it is performed as a
// declaration rather than as a bare store.
func TestDeclarePrintDeclaresALiteralOperand(t *testing.T) {
	literal := printOperand(DeclarePrintOperandIsDeclaredWhereItIsALiteral)
	for _, tc := range []struct{ src, want string }{
		{`typeset -p e=(1 2); echo "[${e[@]}]"`, "typeset -a e=(1 2)\n[1 2]"},
		// The letters that shape the value land, which is what makes this a
		// declaration and not the bare store the reading above makes: the
		// table letter is what decides the kind the literal is read into.
		{`typeset -Ap m=([k]=v); echo "[${m[k]}]"`, "typeset -A m=([k]=v)\n[v]"},
		// And a scope is taken, so the name does not outlive the function.
		{`f() { typeset -p l=(7); }; f; echo "[${l[@]-unset}]"`, "typeset -a l=(7)\n[unset]"},
		// Every store before any listing, as above.
		{`typeset -p c=(1) c=(2)`, "typeset -a c=(2)\ntypeset -a c=(2)"},
		// A scalar operand is a name and not an assignment in this reading:
		// nothing is stored and the listing has nothing to show.
		{`typeset -p s=5; echo "[${s-unset}]"`, "[unset]"},
		{`v=1; typeset -p v`, "v=1"},
	} {
		out, errs, st := declareRun(t, tc.src, literal, Diagnostics{})
		if strings.TrimSuffix(out, "\n") != tc.want || st != 0 || errs != "" {
			t.Errorf("%s = %q (stderr %q, status %d), want %q",
				tc.src, out, errs, st, tc.want)
		}
	}
}

// TestDeclarePrintLiteralOperandTakesNoFreezeOrExport is the measured half of
// the reading above that a shorter one would have got wrong: the letters that
// do not shape the value are dropped, so the array is writable afterwards and
// carries no export attribute.
func TestDeclarePrintLiteralOperandTakesNoFreezeOrExport(t *testing.T) {
	literal := printOperand(DeclarePrintOperandIsDeclaredWhereItIsALiteral)
	out, errs, st := declareRun(t,
		`typeset -rp a=(1 2); a[0]=9; echo "[${a[@]}]"`, literal, Diagnostics{})
	if want := "typeset -a a=(1 2)\n[9 2]"; strings.TrimSuffix(out, "\n") != want || st != 0 || errs != "" {
		t.Errorf("readonly letter = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	out, errs, st = declareRun(t,
		`typeset -xp b=(1 2); typeset -p b`, literal, Diagnostics{})
	if want := "typeset -a b=(1 2)\ntypeset -a b=(1 2)"; strings.TrimSuffix(out, "\n") != want || st != 0 || errs != "" {
		t.Errorf("export letter = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// TestDeclarePrintOperandIsANameAlone: nothing is performed, and the operand
// is a name the listing has nothing for.
func TestDeclarePrintOperandIsANameAlone(t *testing.T) {
	name := func(s *Semantics) {
		s.DeclareListing = DeclareListingBareAssignments
		s.DeclareValueQuoting = ListingQuoteWhenNeededDollar
		// Reported rather than passed over, which is the pairing the column
		// this reading was measured from holds — and it is what keeps the
		// literal operand from landing after the builtin, since a listing
		// that failed gives up the command's own operand assignments.
		s.DeclarePrintReportsAMissingName = Yes
		s.DeclarePrintPerformsItsOperand = DeclarePrintOperandIsANameAlone
	}
	for _, tc := range []struct{ src, want string }{
		{`typeset -p s=5; echo "[${s-unset}]"`, "[unset]"},
		{`typeset -p e=(1 2); echo "[${e[@]-unset}]"`, "[unset]"},
		{`v=1; typeset -p v; echo "[$?]"`, "v=1\n[0]"},
	} {
		out, _, _ := declareRun(t, tc.src, name, Diagnostics{})
		if strings.TrimSuffix(out, "\n") != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestDeclarePrintOperandReportsTheNameItDidNotPerform: the reading that
// performs nothing still reports the operand as a missing name where the
// dialect reports one, and the status is that report's.
func TestDeclarePrintOperandReportsTheNameItDidNotPerform(t *testing.T) {
	reports := func(s *Semantics) {
		s.DeclareListing = DeclareListingBareAssignments
		s.DeclareValueQuoting = ListingQuoteWhenNeededDollar
		s.DeclarePrintReportsAMissingName = Yes
		s.DeclarePrintPerformsItsOperand = DeclarePrintOperandIsANameAlone
	}
	_, errs, st := declareRun(t, `typeset -p s=5`, reports, Diagnostics{})
	if !strings.Contains(errs, "s=5: not found") || st != 1 {
		t.Errorf("stderr %q status %d, want the whole operand reported at 1", errs, st)
	}
}

// TestAHeldListingReportsAfterItsOperandLanded is the same report on the
// reading that performs the literal: the operand that was not performed is
// still reported, and it is reported *beside* the row the performed one
// produced rather than instead of it.
func TestAHeldListingReportsAfterItsOperandLanded(t *testing.T) {
	reports := func(s *Semantics) {
		s.DeclareListing = DeclareListingBareAssignments
		s.DeclareValueQuoting = ListingQuoteWhenNeededDollar
		s.DeclarePrintReportsAMissingName = Yes
		s.DeclarePrintPerformsItsOperand = DeclarePrintOperandIsDeclaredWhereItIsALiteral
	}
	out, errs, st := declareRun(t, `typeset -p s=5 e=(1 2)`, reports, Diagnostics{})
	if !strings.Contains(errs, "s=5: not found") || st != 1 {
		t.Errorf("stderr %q status %d, want the scalar operand reported at 1", errs, st)
	}
	if want := "typeset -a e=(1 2)"; strings.TrimSuffix(out, "\n") != want {
		t.Errorf("stdout %q, want %q", out, want)
	}
}

// TestAnUnansweredDeclarePrintOperandAxisIsRefused: a dialect that has not
// said which of the three it means declares nothing and refuses by name,
// rather than being given one shell's reading.
func TestAnUnansweredDeclarePrintOperandAxisIsRefused(t *testing.T) {
	unanswered := func(s *Semantics) {
		s.DeclareListing = DeclareListingBareAssignments
		s.DeclareValueQuoting = ListingQuoteWhenNeededDollar
		s.DeclarePrintPerformsItsOperand = DeclarePrintOperandUnspecified
	}
	out, errs, _ := declareRun(t, `typeset -p s=5; echo "[$?][${s-unset}]"`, unanswered, Diagnostics{})
	if !strings.Contains(errs, "an operand carrying a value on a `-p` listing") {
		t.Errorf("stderr %q, want the axis named", errs)
	}
	if strings.TrimSuffix(out, "\n") != "[2][unset]" {
		t.Errorf("stdout %q, want 2 and nothing declared", out)
	}
	// And the shape the axis has nothing to say about is unaffected: a
	// listing of a plain name answers from an unanswered dialect.
	out, errs, st := declareRun(t, `v=1; typeset -p v`, unanswered, Diagnostics{})
	if strings.TrimSuffix(out, "\n") != "v=1" || st != 0 || errs != "" {
		t.Errorf("control = %q (stderr %q, status %d), want the ordinary listing", out, errs, st)
	}
}
