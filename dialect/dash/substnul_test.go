// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// **A command substitution drops a NUL**, and drops it before the trailing
// newlines go, so `$(printf 'a\n\0')` is `a`. Measured 2026-10-01
// against dash 0.5.12; see interp.Semantics.NulInAValue.
//
// The script is the one each column of interp.Semantics.NulInAValue was
// measured with: a NUL in the middle of a value, one holding a newline off the
// end, one inside a quoted word and one in an unquoted word with a blank after
// it, and the backquoted spelling.
func TestASubstitutionDropsANul(t *testing.T) {
	src := `v=$(printf 'a\0b'); printf '<%s>' "$v" ${#v}; echo
v=$(printf 'a\n\0'); printf '<%s>' "$v"; echo
printf '<%s>' "x$(printf 'a\0b')y" $(printf 'p\0q r'); echo
v=` + "`" + `printf 'f\0g'` + "`" + `; printf '<%s>' "$v"; echo`
	out, st := runDash(t, t.TempDir(), src)
	if want := "<ab><2>\n<a>\n<xaby><pq><r>\n<fg>\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
