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

// A refused assignment gives up what the shell was running without ending
// the script. It is a third kind of unwinding: `exit` reaches the top and
// stops, `break` and `return` are consumed by a construct, and this unwinds
// past every construct and is consumed at the top-level statement loop.

func abandonRun(t *testing.T, src string) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.ReadonlyReassignmentFatal = No
	sem.ReadonlyReassignmentByDeclarationFatal = No
	var out, errs bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Diagnostics: &Diagnostics{Location: LocationLineWord},
		Dir:         dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// TestARefusedAssignmentGivesUpWhatEnclosesIt covers every shape that holds
// statements, because the answer is the same in all of them and a rule that
// only unwound some of them would look right in most tests.
func TestARefusedAssignmentGivesUpWhatEnclosesIt(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a list on one line", "readonly r=1\nr=2; echo one\necho two"},
		{"an and-or", "readonly r=1\nr=2 && echo one\necho two"},
		{"a loop", "readonly r=1\nfor i in 1 2 3; do r=2; echo one; done\necho two"},
		{"a while", "readonly r=1\nwhile :; do r=2; echo one; break; done\necho two"},
		{"an if", "readonly r=1\nif true; then r=2; echo one; fi\necho two"},
		{"a group", "readonly r=1\n{ r=2; echo one; }\necho two"},
		{"a subshell", "readonly r=1\n( r=2; echo one )\necho two"},
		{"a function body", "readonly r=1\nf() { r=2; echo one; }\nf\necho two"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := abandonRun(t, tc.src)
			// Exactly one complaint: a construct that swallowed the give-up
			// and carried on would run the assignment again, and a loop is
			// where that shows.
			if n := strings.Count(errs, "readonly"); n != 1 {
				t.Errorf("complained %d times, want once: %q", n, errs)
			}
			if strings.Contains(out, "one") {
				t.Errorf("output = %q, want what enclosed it given up", out)
			}
			if !strings.Contains(out, "two") {
				t.Errorf("output = %q, want the shell to carry on", out)
			}
			if st != 0 {
				t.Errorf("status = %d, want the last command's", st)
			}
		})
	}
}

// TestTheLineIsTheBoundary is the measurement that says it is the line and
// not the statement: the same two commands on separate lines both run.
func TestTheLineIsTheBoundary(t *testing.T) {
	out, _, _ := abandonRun(t, "readonly r=1\nr=2\necho one")
	if !strings.Contains(out, "one") {
		t.Errorf("output = %q, want the next line to run", out)
	}
}

// TestGivingUpIsNotExiting, which is the whole reason for a third control
// value: the script keeps going and its status is the next command's.
func TestGivingUpIsNotExiting(t *testing.T) {
	out, _, st := abandonRun(t, "readonly r=1\nr=2; echo one\necho two\ntrue")
	if !strings.Contains(out, "two") || st != 0 {
		t.Errorf("output %q status %d, want the shell still running", out, st)
	}

	// And with nothing after it, the refusal's own status is what stands.
	if _, _, st := abandonRun(t, "readonly r=1\nr=2"); st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestADeclarationGivesUpNothing is the half that keeps this from being "a
// readonly refusal unwinds": the three declaration spellings report and run
// the next command on the same line.
func TestADeclarationGivesUpNothing(t *testing.T) {
	for _, src := range []string{
		"readonly r=1\nexport r=2; echo one\necho two",
		"readonly r=1\nreadonly r=2; echo one\necho two",
	} {
		out, _, _ := abandonRun(t, src)
		if !strings.Contains(out, "one") {
			t.Errorf("%q: output = %q, want the rest of the line to run", src, out)
		}
	}
}
