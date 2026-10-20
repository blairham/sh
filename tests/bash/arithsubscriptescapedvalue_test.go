// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A backslash inside an arithmetic subscript quotes the character after it,
// and the subscript's *second* reading must not undo that.
//
// An arithmetic expression's text is read twice here: once when the
// expression is expanded, and again when a subscript found in it is read as a
// key or as an expression. The first reading took the backslash off, so the
// second one saw a bare `$k` and expanded what the script had escaped —
// `a[\$k]` answered the element the expansion named instead of the one the
// script spelled by hand.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over a script file with standard input on the null device, against bash
// 5.3.20 and, for the two rows that reach it, bash 5.3.15 in the
// digest-pinned image the suite is graded in. With
// `declare -A a; k='a b'; a[$k]=5; a['$k']=11`, so that the escaped spelling
// and the expanded one name *different* elements and a probe with only one of
// them could not tell the readings apart:
//
//	$(( a[\$k] ))      11   the key is `$k`
//	$(( a['\$k'] ))     0   the key is `\$k`, the apostrophes keeping it
//	$(( a[$k] ))        5   the control: an unescaped `$` still expands
//	$(( a["$k"] ))      5   and so does one inside double quotes
//
// This is the line `assoc9.sub` reaches, located 2026-09-23 with a `DEBUG`
// trap carried into the file's children by `BASH_ENV` and then reproduced
// from the panel rather than from the file: an `$(( … ))` over an escaped `$`
// in a table's subscript, which answered 6 here and 1 there (#4179).
func TestAnEscapedDollarInAnArithmeticSubscriptStaysWritten(t *testing.T) {
	const state = "declare -A a\nk='a b'\na[$k]=5\na['$k']=11\n"
	for _, tc := range []struct{ name, read, want string }{
		{"escaped", `echo $(( a[\$k] ))`, "11"},
		{"escaped inside apostrophes", `echo $(( a['\$k'] ))`, "0"},
		{"unescaped", `echo $(( a[$k] ))`, "5"},
		{"inside double quotes", `echo $(( a["$k"] ))`, "5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, state+tc.read+"\n")
			if strings.TrimSpace(out) != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The same rule read as an expression rather than as a key: an indexed name's
// brackets hold arithmetic, and an escaped `$` is not an expansion there
// either — so the expression is handed a `$n` it cannot read and the refusal
// names it.
//
// Measured the same day on bash 5.3.20 with `n=7` and a nine-element `b`:
// `$(( b[\$n] ))` is `arithmetic syntax error` naming `$n`, where expanding
// the escaped `$` answers the element quietly. The quiet wrong answer is the
// reason this row is here beside the table one — the same defect, on the
// reading where it costs a diagnostic rather than an element.
func TestAnEscapedDollarInAnIndexedSubscriptIsNotAnExpansion(t *testing.T) {
	out, st := answersRun(t, "n=7\nb=(0 1 2 3 4 5 6 7 8)\necho $(( b[\\$n] ))\n")
	if !strings.Contains(out, "$n") || !strings.Contains(out, "syntax error") || st == 0 {
		t.Errorf("= %q status %d, want a refusal naming $n at a non-zero status", out, st)
	}
}

// And outside brackets the backslash still comes off, which is what keeps the
// rule to the subscripts it was measured in.
//
// Measured on bash 5.3.20: `(( \$n + 1 ))` is a syntax error whose token is
// `$n + 1` — the backslash gone, the `$` unexpanded — and the refusal text is
// byte-identical here. A rule applied to the whole expression would have left
// the backslash in the token and parted from the reference on a line with no
// subscript in it at all.
func TestOutsideBracketsTheBackslashStillComesOff(t *testing.T) {
	out, st := answersRun(t, "n=7\n(( \\$n + 1 ))\n")
	if !strings.Contains(out, `error token is "$n + 1 "`) || st == 0 {
		t.Errorf("= %q status %d, want the token named without its backslash", out, st)
	}
}

// The refusal that shows the text after the first reading, which is where the
// defect was visible without an element to look up.
//
// Measured on bash 5.3.20, `(( 'a[\$k]' ))` names `'a[\$k]'` — backslash and
// all. Before this it named `'a[$k]'`, which is the first reading having
// already spent the escape.
func TestTheRefusalShowsTheSubscriptWithItsBackslash(t *testing.T) {
	out, _ := answersRun(t, "declare -A a\nk='a b'\na[$k]=5\n(( 'a[\\$k]' ))\n")
	if !strings.Contains(out, `'a[\$k]'`) {
		t.Errorf("= %q, want the refusal to name the text with its backslash", out)
	}
}
