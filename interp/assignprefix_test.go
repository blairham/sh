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
