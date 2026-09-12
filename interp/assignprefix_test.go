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

// An assignment prefixed to a builtin is in effect while the builtin runs and
// is taken back afterward. These pin both halves — the visibility that makes
// `IFS=: read x y` split, and the restore that keeps the prefix transient —
// and the special-builtin persistence axis in both directions.

func prefixAssignRun(t *testing.T, src string, sem Semantics) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Dir: t.TempDir(), Name: "testsh",
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v\nstderr: %s", src, rerr, errs.String())
	}
	return out.String()
}

func TestAPrefixIsVisibleToTheBuiltinItPrefixes(t *testing.T) {
	sem := permissive()
	sem.LastPipelineElementInCurrentShell = Yes
	out := prefixAssignRun(t,
		`echo "a:b" | { IFS=: read x y; echo "[$x][$y]"; }`, sem)
	if !strings.Contains(out, "[a][b]") {
		t.Errorf("output = %q, want the read split on the prefixed IFS", out)
	}
}

func TestAPrefixOnABuiltinIsTakenBackAfterward(t *testing.T) {
	sem := permissive()
	sem.LastPipelineElementInCurrentShell = Yes
	// The probe stays in the same group as the read: the restore is what
	// makes the later unquoted expansion split on whitespace again.
	out := prefixAssignRun(t,
		`echo "a:b" | { IFS=: read x y; v="p q"; set -- $v; echo "n=$#"; }`, sem)
	if !strings.Contains(out, "n=2") {
		t.Errorf("output = %q, want IFS back to whitespace after the read", out)
	}
}

func TestAPrefixedNameThatWasUnsetIsUnsetAgainAfterward(t *testing.T) {
	out := prefixAssignRun(t, `unset v; v=1 read -r _ignored </dev/null; echo "${v-absent}"`,
		permissive())
	if !strings.Contains(out, "absent") {
		t.Errorf("output = %q, want the name unset again after the builtin", out)
	}
}

func TestAPrefixOnASpecialBuiltinFollowsTheAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"persists when the dialect says so", Yes, "[2]"},
		{"is taken back when it says not", No, "[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.AssignmentPrefixPersistsOnSpecialBuiltin = tc.answer
			out := prefixAssignRun(t, `x=1; x=2 export y=3; echo "[$x]"`, sem)
			if !strings.Contains(out, tc.want) {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
		})
	}
}

// An append written as a prefix joins the name's current value rather than
// replacing it, and still leaves the shell's own value alone afterwards.
//
// Unanimous across the panel, and the reason it is worth pinning separately
// from the row above: the operator is on the assignment either way, so a
// prefix route that expands the value and stores it has already lost the
// append without failing anything. The command sees the tail alone, which for
// the idiom this construct exists for — `PATH+=:/x cmd` — is a PATH with one
// entry in it.
func TestAnAppendPrefixJoinsTheValueThatIsThere(t *testing.T) {
	sem := permissive()
	// Taken back afterwards, so the restore and the join are one row: an
	// implementation that stored the joined value on the shell would pass
	// the first half and fail the second.
	sem.AssignmentPrefixPersistsOnSpecialBuiltin = No
	got := prefixAssignRun(t, `v=1; v+=4; v+=5 eval 'echo "[$v]"'; echo "after=[$v]"`, sem)
	if want := "[145]\nafter=[14]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// And an append in front of a name holding nothing is the value alone, which
// is what says the join reads the name rather than assuming one is there.
func TestAnAppendPrefixOverAnUnsetNameIsTheValue(t *testing.T) {
	sem := permissive()
	sem.AssignmentPrefixPersistsOnSpecialBuiltin = No
	got := prefixAssignRun(t, `unset v; v+=5 eval 'echo "[$v]"'`, sem)
	if want := "[5]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
