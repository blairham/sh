// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheElementBeforeTheFirstIsNoFieldWhereFieldsAreKept is #5992: a
// subscript that comes to the place before the first element — `[0]`, or a
// backward search that found nothing — is no word in a quoted expansion that
// keeps its fields, and is unset to an operator. Past the other end it is one
// empty word. Every row measured on zsh 5.9.2 under -f, 2026-10-05; `show`
// prints the count and then each word.
func TestTheElementBeforeTheFirstIsNoFieldWhereFieldsAreKept(t *testing.T) {
	const pre = `show() { printf '%s:' $#; for x in "$@"; do printf '<%s>' "$x"; done; print; }
q=(a b)
`
	for _, tc := range []struct{ src, want string }{
		{`f() { show "${@[0]}"; }; f a`, "0:"},
		{`f() { show "${@[(R)zz]}"; }; f a`, "0:"},
		{`show "${(@)q[0]}"`, "0:"},
		{`show "${(@U)q[0]}"`, "0:"},
		{`show "${(@)q[(R)zz]}"`, "0:"},
		{`show "${(@)q[(R)zz]-D}"`, "1:<D>"},
		{`show "${(@)q[0]-D}"`, "1:<D>"},
		{`show "x${(@)q[0]}y"`, "1:<xy>"},
		// Controls: past the end, a negative count, a search forward, and
		// the spellings that do not keep fields.
		{`show "${(@)q[5]}"`, "1:<>"},
		{`show "${(@)q[-5]}"`, "1:<>"},
		{`show "${(@)q[(r)zz]}"`, "1:<>"},
		{`show "${q[0]}"`, "1:<>"},
		{`show "${(U)q[0]}"`, "1:<>"},
		{`show "${(@)q[(R)a*]}"`, "1:<a>"},
		{`setopt ksharrays; show "${(@)q[0]}"`, "1:<a>"},
	} {
		out, st := runZsh(t, t.TempDir(), pre+tc.src)
		if out != tc.want+"\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
