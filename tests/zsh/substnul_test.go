// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **A command substitution keeps a NUL** as a byte of the value. The
// unquoted word splits at it, because the default IFS holds one. Measured 2026-10-01
// against zsh 5.9.2; see interp.Semantics.NulInAValue.
//
// The script is the one each column of interp.Semantics.NulInAValue was
// measured with: a NUL in the middle of a value, one holding a newline off the
// end, one inside a quoted word and one in an unquoted word with a blank after
// it, and the backquoted spelling.
func TestASubstitutionKeepsANul(t *testing.T) {
	src := `v=$(printf 'a\0b'); printf '<%s>' "$v" ${#v}; echo
v=$(printf 'a\n\0'); printf '<%s>' "$v"; echo
printf '<%s>' "x$(printf 'a\0b')y" $(printf 'p\0q r'); echo
v=` + "`" + `printf 'f\0g'` + "`" + `; printf '<%s>' "$v"; echo`
	out, st := runZsh(t, t.TempDir(), src)
	if want := "<a\x00b><3>\n<a\n\x00>\n<xa\x00by><p><q><r>\n<f\x00g>\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
