// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A line continuation between the two characters of an arithmetic expansion's
// delimiters is read over on the core's answer, so the construct is still
// arithmetic and its value is the expression's.
//
// Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin LC_ALL=C`:
// bash 5.3, bash 3.2 and dash print `[3]` for both delimiter shapes. Before,
// every dialect read a command substitution holding the subshell `( 1 + 2 )`
// and printed a diagnostic about a command called `1` (#3454).
//
// The blank rows are the control that keeps this about the *pair*: with a
// space in front of it the parentheses are no longer adjacent and every column
// reads a command substitution, which is what the panel does and what this
// prints.
func TestAContinuationInsideAnArithmeticExpansionsDelimitersIsReadOver(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the opener", "echo \"[$(\\\n( 1 + 2 ))]\"", "[3]"},
		{"the closer", "echo \"[$(( 1 + 2 )\\\n)]\"", "[3]"},
		{"both ends at once", "echo \"[$(\\\n( 1 + 2 )\\\n)]\"", "[3]"},
		{"an expression with its own nesting", "echo \"[$(\\\n( (1+2)*3 ))]\"", "[9]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"\necho after", nil)
			if want := tc.want + "\nafter"; strings.TrimSpace(out) != want {
				t.Errorf("%q printed %q, want %q", tc.src, strings.TrimSpace(out), want)
			}
		})
	}
}
