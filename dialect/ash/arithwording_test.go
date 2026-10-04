// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// Arithmetic says `arithmetic syntax error` about nearly everything, and
// `malformed ?: operator` about a colon it read as an operator and then found
// standing without a `?`.
//
// Measured 2026-10-04 in the digest-pinned alpine image, BusyBox v1.37.0,
// each expression in `echo $(( … ))` (#5723).
func TestArithmeticWordsAStrayColonApart(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{"1 : 2", "malformed ?: operator"},
		{"1 ? 2 : 3 : 4", "malformed ?: operator"},
		{"1 && 2 : 3", "malformed ?: operator"},
		{"(1 : 2)", "malformed ?: operator"},
		// The reader takes the colon and then wants a value, so these run
		// out of input or find a `?` with no `:` first.
		{"1 :", "arithmetic syntax error"},
		{": 1", "arithmetic syntax error"},
		{"1 ? 2 : 3 : 4 ? 5", "arithmetic syntax error"},
		// A base out of range names no base, and a step with nothing to
		// step names no operator.
		{"1#0", "arithmetic syntax error"},
		{"65#1", "arithmetic syntax error"},
		{"1++", "arithmetic syntax error"},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			out, _ := run(t, "echo $(( "+tc.expr+" ))\n")
			if !strings.Contains(out, tc.want) {
				t.Errorf("$(( %s )) said %q, want %q", tc.expr, out, tc.want)
			}
			if tc.want == "arithmetic syntax error" && strings.Contains(out, "malformed") {
				t.Errorf("$(( %s )) said %q, want no colon complaint", tc.expr, out)
			}
		})
	}
	// And a substring's length reads the same way.
	if out, _ := run(t, "x=abcdef; t=0; echo \"[${x:1:5:t}]\"\n"); !strings.Contains(out, "malformed ?: operator") {
		t.Errorf("${x:1:5:t} said %q, want the colon complaint", out)
	}
}
