// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// **A command substitution drops a NUL**, and drops it before the trailing
// newlines go, so `$(printf 'a\n\0')` is `a`. Measured 2026-10-01 against
// bash 5.3.20; see interp.Semantics.NulInAValue. And it says so on standard
// error, once per substitution that held one and on that command's line —
// five here, the third line having two — see
// interp.Diagnostics.SubstitutionDroppedANul.
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
	out, errs := runBashSplit(t, src)
	if want := "<ab><2>\n<a>\n<xaby><pq><r>\n<fg>\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	const w = "warning: command substitution: ignored null byte in input\n"
	if want := "bash: line 1: " + w + "bash: line 2: " + w + "bash: line 3: " + w + "bash: line 3: " + w + "bash: line 4: " + w; errs != want {
		t.Errorf("stderr %q, want %q", errs, want)
	}
}

// **The `${ … ;}` spelling drops the byte too**, its output being captured
// like any other substitution's: measured on bash 5.3.20, `printf '<%s>' ${
// printf 'a\0b'; }y "${ printf 'c\0d'; }"` is `<aby><cd>`.
func TestACurrentShellSubstitutionDropsANul(t *testing.T) {
	out, _ := runBashSplit(t, `printf '<%s>' ${ printf 'a\0b'; }y "${ printf 'c\0d'; }"; echo`)
	if want := "<aby><cd>\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
