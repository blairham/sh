// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration with no attribute letters and no value: `typeset xyz`.
//
// Two facts have to hold at once and only one of them is a value — the name
// is *unset*, so `${xyz-unset}` fires its default, and the name is
// *recorded*, so the listing writes a row for it. Every path that reached
// this state before had one of the two: an attributed operand leaves a letter
// in an attribute table the listing reads, and a dialect that sets a declared
// name empty leaves a value in the store. The unattributed operand left
// neither, so `typeset -p xyz` answered `xyz: not found` at 1 — which is no
// column's answer, since the shells that record say 0 with a row and the one
// that does not says 0 in silence (#2999).
//
// Whether the record is kept is Semantics.ValuelessDeclarationRecordsTheName,
// and both answers are run below: a test that only ran the recording one
// could not tell a fix from a hardcoding.
func withBareRecord(records Answer) func(*Semantics) {
	return func(s *Semantics) {
		withArrayLetters(No)(s)
		s.ValuelessDeclarationRecordsTheName = records
		// The other half of the three-way split, held flat here so the
		// rows below are about the record alone: the dialect that sets a
		// declared name empty has a value to list and never asks this.
		s.DeclaredNameWithoutValueIsEmpty = No
		// A name with no record draws the complaint, which is the control
		// the listing rows lean on.
		s.DeclarePrintReportsAMissingName = Yes
		// And where the record is kept but not listed, whether the shell
		// still has the name — the third state, which is its own axis and
		// its own suite. Answered `No` here so these rows stay about the
		// record: the `No` answer is the missing-name route the control
		// above then reports. See Semantics.ValuelessRecordIsStillAName.
		s.ValuelessRecordIsStillAName = No
	}
}

// The row and the unset reading together, under both words that reach it.
// `typeset` and `local` build their own declaration sequences, and a record
// written into one loop and not the other is this repository's recurring
// failure — see markDeclaredCompound, which is a function for that reason.
// `declare` is `typeset` under a name a dialect registers, and the third
// loop, the one a subscripted operand takes, always carries a value and so
// never asks. `readonly` names an attribute by being itself.
func TestABareDeclarationIsListedAndStillUnset(t *testing.T) {
	for _, decl := range []string{"typeset xyz", "local xyz"} {
		t.Run(decl, func(t *testing.T) {
			src := "f() { " + decl + `; typeset -p xyz; echo "st=$?"; echo "[${xyz-unset}]"; }
f`
			out, errs, st := declRun(t, src, withBareRecord(Yes), Diagnostics{})
			const want = "declare -- xyz\nst=0\n[unset]\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", decl, out, errs, st, want)
			}
		})
	}
}

// The other answer, and the reason it is a switch: the same three words leave
// nothing behind, so the listing has no row to write and says so.
func TestABareDeclarationMayLeaveNoRecord(t *testing.T) {
	for _, decl := range []string{"typeset xyz", "local xyz"} {
		t.Run(decl, func(t *testing.T) {
			src := "f() { " + decl + `; typeset -p xyz; echo "st=$?"; }
f`
			out, _, _ := declRun(t, src, withBareRecord(No), Diagnostics{})
			const want = "st=1\n"
			if out != want {
				t.Errorf("%s = %q, want %q", decl, out, want)
			}
		})
	}
}

// An attribute letter is a record in every column that has one, so the axis
// must not reach an operand that names one. This is the row that keeps the
// question off the common path: `typeset -i` lists whatever the answer is.
func TestAnAttributedDeclarationListsWhicheverWayTheRecordIsAnswered(t *testing.T) {
	for _, records := range []Answer{Yes, No} {
		src := `f() { typeset -i att; typeset -p att; }
f`
		out, errs, st := declRun(t, src, withBareRecord(records), Diagnostics{})
		const want = "declare -i att\n"
		if out != want || errs != "" || st != 0 {
			t.Errorf("records=%v = %q (stderr %q, status %d), want %q", records, out, errs, st, want)
		}
	}
}

// The record belongs to the cell the declaration made, so a call's own
// declaration does not outlive it. This is the half a fix gets wrong in
// silence: a record kept beside the value tables rather than with the
// attributes the scope already saves passes every row above and leaves the
// name listable at the top level for the rest of the script.
func TestABareDeclarationInACallDoesNotOutliveIt(t *testing.T) {
	src := `f() { typeset lo; typeset -p lo; }
f
typeset -p lo; echo "after=$?"`
	out, _, _ := declRun(t, src, withBareRecord(Yes), Diagnostics{})
	const want = "declare -- lo\nafter=1\n"
	if out != want {
		t.Errorf("= %q, want %q", out, want)
	}
}

// `unset` takes the record away with the attributes, and a declaration after
// it starts the name over rather than finding what was removed. Both
// directions, because a record that merely survived the removal would pass
// the second line and fail the first.
func TestUnsetTakesTheRecordAndADeclarationAfterItBringsItBack(t *testing.T) {
	src := `typeset gone; unset gone; typeset -p gone; echo "gone=$?"
typeset back; unset back; typeset back; typeset -p back; echo "back=$?"`
	out, _, _ := declRun(t, src, withBareRecord(Yes), Diagnostics{})
	const want = "gone=1\ndeclare -- back\nback=0\n"
	if out != want {
		t.Errorf("= %q, want %q", out, want)
	}
}

// What the record turns into once something writes: the row gets its value
// back. A reading that answered from the record first would write the bare
// name here and lose the assignment the script had made.
func TestAnAssignmentAfterABareDeclarationRestoresTheValueToTheRow(t *testing.T) {
	src := `typeset v; v=written; typeset -p v`
	out, errs, st := declRun(t, src, withBareRecord(Yes), Diagnostics{})
	const want = "declare -- v=\"written\"\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The listing with no operands walks its own set of names, so a record that
// only the `-p name` path could find would answer half the question. `grep`
// is not available here, so the row is counted by naming it against a name
// the same listing must not invent.
func TestTheBareListingWalksTheRecordToo(t *testing.T) {
	src := `typeset walked
typeset -p | while read -r line; do case $line in *walked*) echo "row=$line";; esac; done`
	out, errs, st := declRun(t, src, withBareRecord(Yes), Diagnostics{})
	const want = "row=declare -- walked\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The walk the bare listing makes is asked the axis too, and that is not a
// tidiness: the record is kept for every dialect, so a dialect whose listing
// has no row for such a name would collect it, find nothing to say about it,
// and reach the *missing name* path — a second axis, and one the dialects
// with no declaration listing do not answer. It showed as a complaint about
// a name nobody had asked about, from an `export -p` in a function that had
// declared a local.
func TestTheBareListingWalkSkipsTheRecordTheDialectDoesNotKeep(t *testing.T) {
	src := `typeset skipped
typeset -p | while read -r line; do case $line in *skipped*) echo "row=$line";; esac; done
echo "end=$?"`
	out, errs, st := declRun(t, src, func(s *Semantics) {
		withBareRecord(No)(s)
		// The axis the walk would otherwise reach, left unanswered exactly
		// as the two dialects without a declaration listing leave it: if
		// the name gets into the walk, this is what says so.
		s.DeclarePrintReportsAMissingName = Unspecified
	}, Diagnostics{})
	const want = "end=0\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}
