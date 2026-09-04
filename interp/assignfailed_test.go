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

// An assignment that was refused failed, and the assignment that follows it
// must not report success for it. Only reachable in a dialect that reports a
// readonly reassignment and carries on — the three that end the script never
// run the line that would show the status.

func refusedRun(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.ReadonlyReassignmentFatal = No
	sem.ReadonlyReassignmentFatalFromCommandString = No
	sem.ReadonlyReassignmentByDeclarationFatal = No
	var out, errs bytes.Buffer
	dir := t.TempDir()
	r := &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Diagnostics: &Diagnostics{Location: LocationLineWord},
		Dir:         dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String() + errs.String(), st
}

// TestARefusedAssignmentLeavesAFailingStatus is the bug this fixes: the
// refusal set 1 and the next line of the assignment path put 0 back, so a
// script could not tell that anything had gone wrong.
func TestARefusedAssignmentLeavesAFailingStatus(t *testing.T) {
	got, _ := refusedRun(t, "readonly r=1\nr=2\necho st=$?")
	if !strings.Contains(got, "st=1\n") {
		t.Errorf("output = %q, want the refusal's status to stand", got)
	}
	if strings.Contains(got, "st=0") {
		t.Errorf("output = %q, want no success reported for a refusal", got)
	}
}

// TestAnAssignmentThatHappensStillReportsSuccess, which is the rule the fix
// had to keep: only a *refused* assignment fails.
func TestAnAssignmentThatHappensStillReportsSuccess(t *testing.T) {
	got, _ := refusedRun(t, "false\nx=2\necho st=$?")
	if !strings.Contains(got, "st=0\n") {
		t.Errorf("output = %q, want an assignment that happened to report success", got)
	}
}

// TestTheMarkIsCarriedNoFurtherThanItsCommand, so the line after a refusal
// reports its own status rather than inheriting one.
func TestTheMarkIsCarriedNoFurtherThanItsCommand(t *testing.T) {
	got, _ := refusedRun(t, "readonly r=1\nr=2\nx=3\necho st=$?")
	if !strings.Contains(got, "st=0\n") {
		t.Errorf("output = %q, want the assignment after it to report its own success", got)
	}
}

// TestADeclarationReportsARefusedNameToo: `export r=2` would otherwise return
// the builtin's own success and hide a name it had refused to assign.
func TestADeclarationReportsARefusedNameToo(t *testing.T) {
	// `declare` is not a builtin here — it belongs to two of the dialects —
	// so this covers the two that the substrate has. The line is matched
	// whole: "st=127" contains "st=1", and asserting the prefix is how the
	// first version of this test passed while measuring nothing.
	for _, src := range []string{
		"readonly r=1\nexport r=2\necho st=$?",
		"readonly r=1\nreadonly r=2\necho st=$?",
	} {
		got, _ := refusedRun(t, src)
		if !strings.Contains(got, "st=1\n") {
			t.Errorf("%q: output = %q, want the refusal to fail the builtin", src, got)
		}
	}

	// And a declaration that assigns nothing it was refused still succeeds.
	got, _ := refusedRun(t, "export ok=2\necho st=$?")
	if !strings.Contains(got, "st=0\n") {
		t.Errorf("output = %q, want a declaration that worked to report success", got)
	}
}

// TestOnlySomeDeclarationsNameThemselves is why the naming is a set and not
// a flag: one dialect names the builtin for two of its four declaration
// spellings and not for the other two.
func TestOnlySomeDeclarationsNameThemselves(t *testing.T) {
	run := func(src string, names map[string]bool) string {
		t.Helper()
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		sem := permissive()
		sem.ReadonlyReassignmentByDeclarationFatal = No
		var errs bytes.Buffer
		dir := t.TempDir()
		r := &Runner{
			Stdout: &bytes.Buffer{}, Stderr: &errs, Semantics: &sem,
			Diagnostics: &Diagnostics{
				Location:                      LocationLineWord,
				ReadonlyVariable:              "%s: readonly variable",
				ReadonlyVariableInDeclaration: "%[2]s: %[1]s: readonly variable",
				ReadonlyRefusalNamesBuiltin:   names,
			},
			Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
		}
		if _, rerr := r.Run(context.Background(), f); rerr != nil {
			t.Fatalf("run %q: %v", src, rerr)
		}
		return errs.String()
	}

	named := map[string]bool{"readonly": true}
	if got := run("readonly r=1\nreadonly r=2", named); !strings.Contains(got, "readonly: r: readonly variable") {
		t.Errorf("in the set: %q, want the builtin named", got)
	}
	// The same wording is available and is not used, because this builtin is
	// not in the set — which is the half a flag could not express.
	if got := run("readonly r=1\nexport r=2", named); !strings.Contains(got, "line 2: r: readonly variable") {
		t.Errorf("out of the set: %q, want the plain wording", got)
	} else if strings.Contains(got, "export:") {
		t.Errorf("out of the set: %q, want no builtin named", got)
	}
	// And an empty set names none of them, whatever the wording says.
	if got := run("readonly r=1\nreadonly r=2", nil); strings.Contains(got, "readonly: r:") {
		t.Errorf("empty set: %q, want no builtin named", got)
	}
}
