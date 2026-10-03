// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestTraceAssignmentListIsWrittenAsItGoes pins the axis by name. A refusal
// from a store lands inside the list's one line under Yes, after the words
// already written and before the line's newline. Under No the line is written
// whole once the list is done. A list with nothing to say in the middle is
// the same line either way. See interp/xtraceopenline.go.
func TestTraceAssignmentListIsWrittenAsItGoes(t *testing.T) {
	const refused = `readonly r; set -x; a=1 r=2 j=3`
	sem := permissive()
	sem.TraceAssignmentsSeparately = No
	diag := Diagnostics{ReadonlyVariable: "%s: is read only"}

	sem.TraceAssignmentListIsWrittenAsItGoes = Yes
	if got, want := traceOf(t, refused, sem, diag), "+ a=1 r=2 sh: r: is read only\n\n"; got != want {
		t.Errorf("Yes: got %q, want %q", got, want)
	}
	if got, want := traceOf(t, `set -x; a=1 b=$(echo x)`, sem, Diagnostics{}), "+ echo x\n+ a=1 b=x \n"; got != want {
		t.Errorf("Yes, nothing in the middle: got %q, want %q", got, want)
	}

	// Under No only the order is asserted: what a refused list's line then
	// holds is #5509.
	sem.TraceAssignmentListIsWrittenAsItGoes = No
	if got, want := traceOf(t, refused, sem, diag), "sh: r: is read only\n+ a=1 r=2"; !strings.HasPrefix(got, want) {
		t.Errorf("No: got %q, want it to start %q", got, want)
	}
	if got, want := traceOf(t, `set -x; a=1 b=$(echo x)`, sem, Diagnostics{}), "+ echo x\n+ a=1 b=x\n"; got != want {
		t.Errorf("No, nothing in the middle: got %q, want %q", got, want)
	}
}
