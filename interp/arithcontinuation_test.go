// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A backslash-newline inside an arithmetic expression is a line continuation
// on every route into one, and the script goes on past it. Measured
// 2026-09-16 from script files: bash 5.3, bash 3.2, zsh 5.9.2 and ksh93u+
// print every line below, and dash every line of the ones it has the
// construct for (#3442). Before, the first of them was an arithmetic syntax
// error quoting a backslash and a newline, and the script stopped there.
//
// The here-document row is the route the parser does not see: an unquoted
// body is read into words when it runs, through the same scanner.
func TestALineContinuationInArithmeticIsRemovedOnEveryRoute(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an expansion", "echo \"[$(( 1\\\n+1 ))]\"", "[2]"},
		{"an assignment's value", "x=$((3 \\\n+ 3)); echo \"[$x]\"", "[6]"},
		{"an operator split in two", "echo \"[$(( 1 <\\\n< 3 ))]\"", "[8]"},
		{"a name split in two", "abc=7; echo \"[$(( ab\\\nc + 1 ))]\"", "[8]"},
		{"a subscript", "a=(4 5 6); echo \"[$(( a[1\\\n] ))]\"", "[5]"},
		{"a braced parameter", "x=5; echo \"[$(( ${x\\\n} + 1 ))]\"", "[6]"},
		{"a parameter the continuation joins", "x=1; echo \"[$(( $x\\\n+1 ))]\"", "[2]"},
		{"the command", "(( 2\\\n+2 )) && echo '[ok]'", "[ok]"},
		{"the for header", "for ((i=0; i<1\\\n; i++)); do echo \"[$i]\"; done", "[0]"},
		{"an unquoted here-document body", "cat <<E\n[$(( 1\\\n2 + 1 ))]\nE", "[13]"},
		// And the control: a quoted delimiter leaves the body alone, the
		// continuation and the expansion with it.
		{"a quoted here-document body", "cat <<'E'\n[$(( 1\\\n2 ))]\nE", "[$(( 1\\\n2 ))]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"\necho after", nil)
			if want := tc.want + "\nafter"; strings.TrimSpace(out) != want {
				t.Errorf("%q printed %q, want %q", tc.src, strings.TrimSpace(out), want)
			}
		})
	}
}
