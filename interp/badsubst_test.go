// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// A deferred bad substitution is diagnosed only when the expansion is
// reached, with the fatal-status axis deciding the number it carries.

func badSubstRun(t *testing.T, src string, statusIsOne Answer) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.FatalErrorStatusIsOne = statusIsOne
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &errs, Semantics: &sem, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

func TestABadSubstitutionInABranchNeverTakenIsSilent(t *testing.T) {
	out, errs, st := badSubstRun(t, `if false; then echo "${foo ~}"; fi; echo ok`, Yes)
	if !strings.Contains(out, "ok") || errs != "" || st != 0 {
		t.Errorf("out=%q errs=%q st=%d, want a silent ok", out, errs, st)
	}
}

func TestABadSubstitutionReachedIsDiagnosedAndFatal(t *testing.T) {
	out, errs, st := badSubstRun(t, `echo "${foo ~}"; echo after`, Yes)
	if !strings.Contains(errs, "${foo ~}: bad substitution") {
		t.Errorf("stderr = %q, want the construct named", errs)
	}
	if strings.Contains(out, "after") {
		t.Errorf("stdout = %q, want the rest abandoned", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want the fatal status the axis chose", st)
	}
}

func TestTheFatalStatusAxisReachesABadSubstitution(t *testing.T) {
	_, _, st := badSubstRun(t, `echo "${foo ~}"`, No)
	if st != 2 {
		t.Errorf("status = %d, want 2 when the axis says fatal errors are 2", st)
	}
}
