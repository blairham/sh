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

// How far a given-up line reaches out of borrowed text, which is two answers
// and not one: `eval` and a sourced file are a *file* of statements, so a
// statement that gives up its line there costs that line and the text carries
// on — except inside a subshell, where the give-up passes straight through
// and the subshell ends with it.
//
// Every row here is shaped to say which of those it is asking. A statement
// that gives up directly inside a subshell ends it whatever this rule says,
// because nothing between it and the subshell's own list catches the give-up;
// so the discriminating shape is a **two-line** `eval` inside a subshell with
// a statement after the `eval`. Both halves matter: with one line in the text
// there is nothing for a resume to run, and with nothing after the `eval`
// there is nothing for the subshell to lose.
//
// Not an axis, for the reason the code gives: every route a script has to a
// given-up line is answered the abandoning way by one preset alone, so the
// panel has one column that can be asked. See interp/source.go (#2747).

// abandonSubshellRun runs src and returns what the shell wrote and what it
// exited with. The give-up is a reassignment to a readonly name reported
// rather than fatal, which is the shape the one answering column is in.
func abandonSubshellRun(t *testing.T, src string) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.ReadonlyReassignmentFatal = No
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

func TestAGivenUpLineLeavesBorrowedTextOnlyInsideASubshell(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The row that moved. Nothing of the `eval` past the failing line
		// runs, and nothing of the subshell after the `eval` runs either.
		{
			"borrowed text inside a subshell loses the subshell",
			"readonly r=1\n(\neval 'r=2\necho inner'\necho sub\n)\necho after\n",
			"after\n",
		},
		// The same text at the top level, which says the answer is about the
		// subshell rather than about `eval`: the give-up costs its line and
		// the borrowed text goes on to its next one.
		{
			"the same text at the top level keeps reading",
			"readonly r=1\neval 'r=2\necho inner'\necho after\n",
			"inner\nafter\n",
		},
		// A sourced file is borrowed text too, and the sourced case is the
		// one a `.` in a subshell reaches. Written with `eval` inside an
		// `eval` rather than a file so the row needs no scratch directory:
		// the inner text is what gives up, and the outer one is what would
		// have resumed.
		{
			"borrowed text inside borrowed text inside a subshell",
			"readonly r=1\n(\neval \"eval 'r=2\necho inner'\necho mid\"\necho sub\n)\necho after\n",
			"after\n",
		},
		// The control that keeps the rule from reading as "a subshell dies
		// on a give-up", which was already true and is not what moved: with
		// no borrowed text between, both readings end the subshell.
		{
			"a give-up with no borrowed text between",
			"readonly r=1\n(\nr=2\necho inner\necho sub\n)\necho after\n",
			"after\n",
		},
		// And the give-up still costs only its own line inside a function,
		// which is the enclosing shape that is *not* a subshell.
		{
			"a function body is not a subshell",
			"readonly r=1\nf(){ eval 'r=2\necho inner'\necho fn\n}\nf\necho after\n",
			"inner\nfn\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := abandonSubshellRun(t, tc.src)
			if out != tc.want {
				t.Errorf("wrote %q, want %q", out, tc.want)
			}
			if !strings.Contains(errs, "r") {
				t.Errorf("said %q, want the refusal named", errs)
			}
			if st != 0 {
				t.Errorf("exited %d, want 0 — the script itself did not fail", st)
			}
		})
	}
}
