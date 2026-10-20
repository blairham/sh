// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A `]` written inside a quoted run does not end a `${a[ … ]}` subscript, so
// a key with a bracket in it reads back the way it was written.
//
// The write side has always quoted — it is the lexer's word boundary, and
// syntax.Dialect.SubscriptSpansSeparators measured it — while the read side
// stopped at the quoted bracket and handed the rest to the operator scan,
// which called the whole expansion a bad substitution. Measured against bash
// 5.3.20 (2026-09-15): every line below answers `1`.
func TestAQuotedBracketDoesNotEndASubscript(t *testing.T) {
	for _, src := range []string{
		`declare -A a; a['x]y']=1; echo "${a['x]y']}"`,
		`declare -A a; a['x]y']=1; echo "${a["x]y"]}"`,
		`declare -A a; a['x]y']=1; echo "${a[x\]y]}"`,
		// A length and a default reach the same scan, so this is the scan
		// rather than one operator's reading of it.
		`declare -A a; a['x]y']=1; echo "${#a['x]y']}"`,
		`declare -A a; a['x]y']=1; echo "${a['no]key']-1}"`,
		// A `"` inside the double-quoted run is quoted by the backslash in
		// front of it and does not end that run early.
		`declare -A a; a['x"y]z']=1; echo "${a["x\"y]z"]}"`,
	} {
		out, st := runBash(t, t.TempDir(), src)
		if out != "1\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, "1\n")
		}
	}
}

// And the ordinary spelling is untouched: a subscript with no quoting in it
// still ends at its own bracket, and one bracketed expansion inside another
// still closes at the matching one.
func TestAnUnquotedSubscriptStillEndsAtItsBracket(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y z); echo "${a[1]}"`, "y\n"},
		{`a=(x y z); b=(0 1); echo "${a[b[1]]}"`, "y\n"},
		{`unset t; echo "[${t[@]+set}]"`, "[]\n"},
		{`t=(p); echo "[${t[@]+${t[@]}}]"`, "[p]\n"},
	} {
		out, st := runBash(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}
