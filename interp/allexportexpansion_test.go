// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `set -a` reaching a name an expansion or arithmetic assigns, both answers of
// Semantics.AllexportMarksAnExpansionsAssignment. Each row stores into a name
// nothing has exported and reads the child's environment back; `x=9` is the
// control, exported under either answer.
func TestAllexportMarksAnExpansionsAssignment(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, store, as string }{
		{"a default assignment", `: ${w:=1}`, "w"},
		{"an unset-only assignment", `: ${w=1}`, "w"},
		{"an arithmetic expansion", `: $(( w = 1 ))`, "w"},
		{"an increment", `: $(( w++ ))`, "w"},
		{"an arithmetic command", `(( w = 1 ))`, "w"},
		{"let", `let w=1`, "w"},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				sem := permissive()
				sem.AllexportMarksAnExpansionsAssignment = answer
				out, st := run(t, `set -a; x=9; `+tc.store+`; set +a; env | grep -E '^(x|`+tc.as+`)='`, withSem(sem))
				if st != 0 || !strings.Contains(out, "x=9\n") {
					t.Fatalf("out=%q st=%d: the control did not export", out, st)
				}
				if got := strings.Contains(out, tc.as+"="); got != (answer == Yes) {
					t.Errorf("answer %v: out=%q, exported=%v", answer, out, got)
				}
			})
		}
	}
}

// Nothing is asked without `set -a`, so the axis is consulted only where the
// shells part: the same stores under CoreSemantics, which answers nothing about
// it, run silently and export nothing.
func TestAnExpansionsAssignmentWithoutAllexportAsksNothing(t *testing.T) {
	t.Parallel()
	out, st := run(t, `: ${w:=1}; : $(( m = 2 )); echo $w $m; env | grep -E '^(w|m)='`, withSem(CoreSemantics()))
	if st != 1 || out != "1 2\n" {
		t.Errorf("out=%q st=%d, want the values, nothing exported and no refusal", out, st)
	}
}
