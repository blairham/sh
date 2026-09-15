// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The exception one dialect carves out of the boundary a special builtin draws
// around text it is running: every fatal error is caught there except the ones
// a builtin raised about **how it was called** (#2950).
//
// Both answers are asserted for every case, and a non-usage error is asserted
// beside every usage one. A test that only ran the escaping answer would pass
// against an implementation that caught nothing, and one that only raised a
// usage error would pass against an implementation that caught nothing *here*
// — which is the shape of the bug this is about, read from the other side.

// usageSemantics is permissive() with the two axes these tests move, plus the
// two fatalities they need to have an error to raise at all.
func usageSemantics(escapes Answer) Semantics {
	s := permissive()
	s.FatalErrorStatusIsOne = Yes
	s.FatalErrorEndsBorrowedTextOnly = Yes
	s.BuiltinUsageErrorEscapesBorrowedText = escapes
	// The usage error: a bad option to a special builtin, which POSIX makes
	// fatal and which permissive() leaves alone.
	s.BadOptionToSpecialBuiltinFatal = Yes
	// And the error that is not one, raised by a builtin all the same.
	s.ReadonlyReassignmentFatal = Yes
	return s
}

// usageDiagnostics keeps every complaint these tests provoke to one short
// line, so an assertion can be the whole of the output.
func usageDiagnostics() Diagnostics {
	return Diagnostics{
		BuiltinBadOption: "%[1]s: %[2]s: invalid option",
		ReadonlyVariable: "%s: is read only",
	}
}

// TestAUsageErrorEscapesBorrowedTextWhereTheDialectSaysSo drives the axis
// through both of the builtins the boundary belongs to.
//
// The assertion is the whole output and the run's status together. "Nothing
// after it ran" is the half that distinguishes the two answers — the complaint
// itself is written either way, and so is the 2 the builtin reported, since
// nothing here overrides what a caught error is reported as. The status the
// *run* ends at is the second half: 0 where the script reached its last line
// and 2 where the error took the shell with it.
func TestAUsageErrorEscapesBorrowedTextWhereTheDialectSaysSo(t *testing.T) {
	tests := []struct {
		name, src string
	}{
		{"eval", "eval 'export -Q x'\necho \"AFTER st=$?\"\n"},
		{"dot", ". ./u.sh\necho \"AFTER st=$?\"\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Caught: the builtin reports, and the script carries on at the
			// command after it.
			dir := t.TempDir()
			write(t, dir, "u.sh", "export -Q x\necho IN-AFTER\n")
			out, st := sourceRun(t, dir, tc.src, usageSemantics(No), usageDiagnostics())
			const caught = "testsh: export: -Q: invalid option\nAFTER st=2\n"
			if out != caught {
				t.Errorf("caught: got %q, want %q", out, caught)
			}
			if st != 0 {
				t.Errorf("caught: status = %d, want 0: the shell was not asked to stop", st)
			}

			// Escaping: the same complaint, and then nothing — neither the
			// rest of the dot script nor the line after the builtin.
			dir = t.TempDir()
			write(t, dir, "u.sh", "export -Q x\necho IN-AFTER\n")
			out, st = sourceRun(t, dir, tc.src, usageSemantics(Yes), usageDiagnostics())
			const escaped = "testsh: export: -Q: invalid option\n"
			if out != escaped {
				t.Errorf("escaping: got %q, want %q", out, escaped)
			}
			if st != 2 {
				t.Errorf("escaping: status = %d, want the bad option's 2", st)
			}
		})
	}
}

// TestANonUsageErrorIsStillCaughtWhereAUsageErrorEscapes is the discriminator.
//
// A readonly reassignment is raised *by a builtin* too, and it is caught in
// the shell this axis was measured on. So an implementation that had read the
// line as "an error inside a builtin" rather than as "an error about the call"
// passes every assertion above and fails here.
func TestANonUsageErrorIsStillCaughtWhereAUsageErrorEscapes(t *testing.T) {
	dir := t.TempDir()
	out, st := sourceRun(t, dir,
		"readonly r=1\neval 'r=2'\necho \"AFTER st=$?\"\n",
		usageSemantics(Yes), usageDiagnostics())
	const want = "testsh: r: is read only\nAFTER st=1\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0: the boundary still catches this one", st)
	}
}

// TestExitInsideBorrowedTextIsNotAUsageError guards the half that keeps this
// from turning `exit` into something a boundary decides about. Both answers
// are run, because a catch that consumed controlExit without asking which kind
// it is holding is invisible under the answer that lets everything out.
func TestExitInsideBorrowedTextIsNotAUsageError(t *testing.T) {
	for _, escapes := range []Answer{Yes, No} {
		dir := t.TempDir()
		out, st := sourceRun(t, dir, "eval 'exit 7'\necho NOT-REACHED\n",
			usageSemantics(escapes), usageDiagnostics())
		if out != "" {
			t.Errorf("axis %v: got %q, want the shell to stop at the exit", escapes, out)
		}
		if st != 7 {
			t.Errorf("axis %v: status = %d, want 7", escapes, st)
		}
	}
}
