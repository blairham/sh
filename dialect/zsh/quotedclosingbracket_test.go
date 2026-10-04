// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A quoted `]`, or one a value supplied, cannot close a bracket the source
// opened here, so the pattern is refused as unterminated rather than matched
// against the filesystem and missed. Measured 2026-10-04 on zsh 5.9.2 in a
// directory where nothing matches; the controls keep a bracket the source
// closed, and a value's `]` with no bracket in front of it.
func TestAQuotedClosingBracketClosesNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"written quoted", `echo nos[a"]"`, "bad pattern: nos[a]"},
		{"from a value", `k="]"; echo nos[a$k`, "bad pattern: nos[a]"},
		{"from a value through an arithmetic subscript", `a=(9 8 7); k="1]"; echo "$(( a[$k ))"`, "bad pattern: a[1]"},
		{"control: the source closes it", `echo nos[a"]"]`, "no matches found: nos[a]]"},
		{"control: no bracket to close", `k="a]"; echo $k`, "a]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, errs := runZshSplit(t, t.TempDir(), tc.src)
			if !strings.Contains(out+errs, tc.want) {
				t.Errorf("stdout %q stderr %q, want %q", out, errs, tc.want)
			}
		})
	}
}
