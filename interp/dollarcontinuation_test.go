// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A line continuation between a `$` and what it introduces is removed and the
// `$` introduces what stands behind it, on every route into a word.
//
// Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin LC_ALL=C`:
// bash 5.3, bash 3.2 and dash print every line below, and the unquoted braced
// shape is printed by the whole panel. Before, the `$` was left as text in
// every dialect and the line printed `[${x}]` (#3457).
//
// The last two rows are the floor: a `$` reaches a *form*, so a pair with a
// blank or the end of the word behind it leaves the `$` alone — and those two
// are unanimous, so a change that simply deleted the pair would break them.
func TestADollarReachesAcrossALineContinuationOnEveryRoute(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a braced parameter", "x=5; echo \"[$\\\n{x}]\"", "[5]"},
		{"a bare parameter", "x=5; echo \"[$\\\nx]\"", "[5]"},
		{"a braced parameter unquoted", "x=5; printf '[%s]\\n' $\\\n{x}", "[5]"},
		{"a bare parameter unquoted", "x=5; printf '[%s]\\n' $\\\nx", "[5]"},
		{"a command substitution", "echo \"[$\\\n(echo hi)]\"", "[hi]"},
		{"arithmetic", "echo \"[$\\\n((1+2))]\"", "[3]"},
		{"a special parameter", "set -- a b; echo \"[$\\\n#]\"", "[2]"},
		{"an assignment's value", "x=5; y=$\\\n{x}; echo \"[$y]\"", "[5]"},
		{"an unquoted here-document body", "x=5; cat <<E\n[$\\\nx][$\\\n{x}]\nE", "[5][5]"},

		{"a blank behind the pair", "echo \"[$\\\n x]\"", "[$ x]"},
		{"the end of the word behind it", "echo \"[$\\\n]\"", "[$]"},
		// The control: a quoted delimiter leaves the body alone, the pair
		// and the `$` with it.
		{"a quoted here-document body", "x=5; cat <<'E'\n[$\\\nx]\nE", "[$\\\nx]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"\necho after", nil)
			if want := tc.want + "\nafter"; strings.TrimSpace(out) != want {
				t.Errorf("%q printed %q, want %q", tc.src, strings.TrimSpace(out), want)
			}
		})
	}
}
