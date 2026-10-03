// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestTraceAssignmentListIsWrittenAsItGoes pins the axis by name. A refusal
// from a store lands inside the list's one line under Yes, after the words
// already written and before the line's newline. Under No the line is written
// whole once the list is done. Under Yes a substitution in a value writes a
// copy of the line so far ahead of its own (#5546). See
// interp/xtraceopenline.go.
func TestTraceAssignmentListIsWrittenAsItGoes(t *testing.T) {
	const refused = `readonly r; set -x; a=1 r=2 j=3`
	sem := permissive()
	sem.TraceAssignmentsSeparately = No
	diag := Diagnostics{ReadonlyVariable: "%s: is read only"}

	sem.TraceAssignmentListIsWrittenAsItGoes = Yes
	if got, want := traceOf(t, refused, sem, diag), "+ a=1 r=2 sh: r: is read only\n\n"; got != want {
		t.Errorf("Yes: got %q, want %q", got, want)
	}
	if got, want := traceOf(t, `set -x; a=1 b=$(echo x)`, sem, Diagnostics{}), "+ a=1 b=+ echo x\n+ a=1 b=x \n"; got != want {
		t.Errorf("Yes, a substitution: got %q, want %q", got, want)
	}

	// Under No the line waits for the whole list, and a refused list never
	// gets there: the refusal is all that is written (#5509).
	sem.TraceAssignmentListIsWrittenAsItGoes = No
	if got, want := traceOf(t, refused, sem, diag), "sh: r: is read only\n"; got != want {
		t.Errorf("No: got %q, want %q", got, want)
	}
	if got, want := traceOf(t, `set -x; a=1 b=$(echo x)`, sem, Diagnostics{}), "+ echo x\n+ a=1 b=x\n"; got != want {
		t.Errorf("No, nothing in the middle: got %q, want %q", got, want)
	}
}
