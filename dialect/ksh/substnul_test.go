// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **A substituted NUL ends the field it lands in** — the `y` written after
// the substitution is gone too, while a blank after the NUL still makes a field
// of its own — and the trailing newlines go first, so a newline the NUL held
// off the end stays. Measured 2026-10-01
// against ksh93u+ 2012-08-01; see interp.Semantics.NulInAValue.
//
// The script is the one each column of interp.Semantics.NulInAValue was
// measured with: a NUL in the middle of a value, one holding a newline off the
// end, one inside a quoted word and one in an unquoted word with a blank after
// it, and the backquoted spelling.
func TestASubstitutionCutsAtANul(t *testing.T) {
	src := `v=$(printf 'a\0b'); printf '<%s>' "$v" ${#v}; echo
v=$(printf 'a\n\0'); printf '<%s>' "$v"; echo
printf '<%s>' "x$(printf 'a\0b')y" $(printf 'p\0q r'); echo
v=` + "`" + `printf 'f\0g'` + "`" + `; printf '<%s>' "$v"; echo`
	out, st := runKsh(t, t.TempDir(), src)
	if want := "<a><1>\n<a\n>\n<xa><p><r>\n<f>\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// **The `${ … ;}` spelling cuts the same way**, its output being captured
// like any other substitution's: measured on ksh93u+, `printf '<%s>' ${ printf
// 'a\0b'; }y "${ printf 'c\0d'; }"` is `<a><c>`.
func TestACurrentShellSubstitutionCutsAtANul(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `printf '<%s>' ${ printf 'a\0b'; }y "${ printf 'c\0d'; }"; echo`)
	if want := "<a><c>\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
