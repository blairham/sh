// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The routes past specialParam that read or store a position by its digits:
// a store through `::=`, the zip operator's operand, and arithmetic. Each
// panicked on an index past 2^63-1 (#6849); each now reads it as past the end
// and stores nothing. What zsh 5.9.2 writes on stderr beside it is #6808's.
func TestEveryRouteToAPositionTooWideIsPastTheEnd(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`set -- a b; : ${9223372036854775808::=x}; print -r -- "$# ok"`, "2 ok\n"},
		{`set -- a b; a=(1 2); print -r -- ${a:^9223372036854775808}`, "1\n"},
		{`set -- a b; print -r -- $(( $9223372036854775808 ))`, "0\n"},
		{`set -- a b; print -r -- ${#9223372036854775808}`, "0\n"},
	} {
		got, st := runZsh(t, t.TempDir(), tc.src+" 2>/dev/null")
		if strings.Contains(got, "internal error") || !strings.HasSuffix(got, tc.want) || st != 0 {
			t.Errorf("%s: %q at %d, want it to end %q at 0", tc.src, got, st, tc.want)
		}
	}
}
