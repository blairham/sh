// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `-f` declaration that *marks* a name to be defined later instead of
// listing it — see Semantics.FunctionLettersThatMarkUndefined and
// Runner.SetFunctionMarkedUndefined, and #1753.
//
// Named for the axis and the seam, never for a shell. The hook here records
// what it was handed rather than autoloading anything: what this package
// decides is which lines reach it and with what, and the marking itself is
// the dialect's.

func markRun(t *testing.T, src, letters string) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		sem.DeclareOptions = "afgpruUz"
		sem.DeclareOptionsWithoutEffect = "z"
		sem.FunctionLettersThatMarkUndefined = letters
		sem.FunctionNamesUnderPlus = No
		r.Semantics = &sem
		r.SetFunctionMarkedUndefined(func(_ *Runner, names []string, got string) int {
			_, _ = fmt.Fprintf(r.Out(), "marked [%s] with [%s]\n", strings.Join(names, " "), got)
			return 0
		})
	})
}

// The headline: the same line is a marking under one answer and a listing
// under the other, and nothing about the line itself says which.
func TestTheLettersThatMarkAnUndefinedFunctionAreAnAxis(t *testing.T) {
	const src = `f(){ :; }; typeset -fu f`
	out, st := markRun(t, src, "uU")
	if want := "marked [f] with [u]\n"; out != want || st != 0 {
		t.Errorf("marking: got %q/%d, want %q", out, st, want)
	}
	out, st = markRun(t, src, "")
	if !strings.HasPrefix(out, "f () ") || st != 0 {
		t.Errorf("listing: got %q/%d, want the body written out", out, st)
	}
}

// The letters go across whole and in the order they were written, `f`
// excepted — which is what lets a dialect record one of them on the stub it
// writes without this package knowing what any of them mean.
func TestTheMarkingSeamIsHandedTheLettersAsWritten(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -fu nm`, "marked [nm] with [u]\n"},
		{`typeset -fU nm`, "marked [nm] with [U]\n"},
		{`typeset -fuz nm`, "marked [nm] with [uz]\n"},
		{`typeset -fzu nm`, "marked [nm] with [zu]\n"},
		{`typeset -f -u -z nm`, "marked [nm] with [uz]\n"},
		{`typeset -fu a b`, "marked [a b] with [u]\n"},
	} {
		out, st := markRun(t, tc.src, "uU")
		if out != tc.want || st != 0 {
			t.Errorf("%s: got %q/%d, want %q", tc.src, out, st, tc.want)
		}
	}
}

// A letter the dialect records but does not count as a *marking* letter does
// not start one. The line is still a listing, which is the reading that says
// the two roles are separate: `z` decorates a marking and cannot begin one.
func TestADecoratingLetterAloneIsStillAListing(t *testing.T) {
	out, st := markRun(t, `f(){ :; }; typeset -fz f`, "uU")
	if !strings.HasPrefix(out, "f () ") || st != 0 {
		t.Errorf("got %q/%d, want the body written out", out, st)
	}
}

// Two lines that look like markings and are not. Operands are required —
// a `-f` line with none is the listing it has always been — and so is the
// minus sign, because the plus spelling is measured to be a refusal in the
// shell that has the letters rather than a marking under another name.
func TestOnlyAMinusSignedLineWithOperandsMarks(t *testing.T) {
	for _, src := range []string{
		`f(){ :; }; typeset -fu`,
		`f(){ :; }; typeset +fu f`,
	} {
		out, st := markRun(t, src, "uU")
		if strings.Contains(out, "marked") || st != 0 {
			t.Errorf("%s: got %q/%d, want no marking", src, out, st)
		}
	}
}

// And a dialect that names the letters without installing the seam gets the
// listing rather than a panic or a silence: the field says which lines are
// not listings, and only the hook can make one into anything else.
func TestNamingTheLettersWithoutTheSeamStillLists(t *testing.T) {
	out, st := run(t, `f(){ :; }; typeset -fu f`, func(r *Runner) {
		sem := CoreSemantics()
		sem.DeclareOptions = "afgpruUz"
		sem.FunctionLettersThatMarkUndefined = "uU"
		r.Semantics = &sem
	})
	if !strings.HasPrefix(out, "f () ") || st != 0 {
		t.Errorf("got %q/%d, want the body written out", out, st)
	}
}
