// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `[[:ascii:]]`, which is the one character-class name this shell has beyond
// the twelve POSIX ones — and the one place its roster is not empty.
//
// Measured 2026-09-10 on bash 5.3.15 and bash 3.2.57 alike: `a` is in it and
// a two-byte `é` is not, so it is a statement about the byte rather than
// another spelling of `print`. ksh93 and dash have no such name and match
// nothing with it, silently, which is what every shell here does with a name
// it does not know (#1721).
func TestTheAsciiCharacterClass(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ a = [[:ascii:]] ]] && echo hit || echo miss`, "hit"},
		{`[[ " " = [[:ascii:]] ]] && echo hit || echo miss`, "hit"},
		{`[[ é = [[:ascii:]] ]] && echo hit || echo miss`, "miss"},
		// The names are case-sensitive, and nothing else joined the roster:
		// the classes one other shell keeps are unknown here and match
		// nothing, at status 0 and in silence.
		{`[[ a = [[:ASCII:]] ]] && echo hit || echo miss`, "miss"},
		{`[[ a = [[:IDENT:]] ]] && echo hit || echo miss; echo "st=$?"`, "miss\nst=0"},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}
