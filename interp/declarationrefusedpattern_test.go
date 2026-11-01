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

// A pattern refused behind a declaration's assignment operand is a failure
// among the declaration's words, so it leaves `$?` alone where
// Semantics.FailedExpansionInADeclarationOrCaseSetsNoStatus says a failure
// there does (#7005). `false` in front leaves 1 and the fatal status is 2
// here, so the two readings are two numbers. Tests name the axis and never a
// shell.
func declarationRefusalStatus(t *testing.T, src string, keeps Answer) int {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.GlobNoMatchIsError = Yes
	sem.FatalErrorStatusIsOne = No
	sem.FailedExpansionInADeclarationOrCaseSetsNoStatus = keeps
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Route: RouteCommandString, Stdout: &buf, Stderr: &buf,
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "nomatch*zz") {
		t.Fatalf("%q wrote %q, which refuses no pattern", src, buf.String())
	}
	return st
}

func TestAPatternRefusedBehindADeclarationsAssignmentCanLeaveTheStatus(t *testing.T) {
	t.Parallel()
	const src = "false; export y=1 nomatch*zz\n"
	if st := declarationRefusalStatus(t, src, Yes); st != 1 {
		t.Errorf("%q with the axis on = %d, want the 1 `false` left", src, st)
	}
	if st := declarationRefusalStatus(t, src, No); st != 2 {
		t.Errorf("%q with the axis off = %d, want the fatal 2", src, st)
	}
}

// With the pattern in front of the assignment it is not behind one, and the
// fatal status is set on either side of the axis.
func TestAPatternRefusedAheadOfADeclarationsAssignmentSetsTheStatus(t *testing.T) {
	t.Parallel()
	const src = "false; export nomatch*zz y=1\n"
	for _, keeps := range []Answer{Yes, No} {
		if st := declarationRefusalStatus(t, src, keeps); st != 2 {
			t.Errorf("%q = %d, want the fatal 2", src, st)
		}
	}
}
