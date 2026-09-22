// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `unset a[$k]` writes its own brackets, so whatever the expansion between
// them came to is the key — a bracket in it, a backslash in it, a quote
// character in it.
//
// Measured 2026-09-22 against bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// over a table holding the keys `x]y`, `]`, `[`, `\`, `x` and `'q'`: every
// one of them is removed by the unquoted spelling, and the plain `q` beside
// the quoted key is left alone.
//
// All of them were silently left standing here, at status 0 with the table
// unchanged, because the operand was scanned for a balanced pair of brackets
// as though a builtin had been handed the text — so the `]` the key is made
// of ended the subscript — and because the quoting the scan stepped over was
// then taken off a key it belongs to.
//
// The quoted spelling is the control and it removes nothing in bash either:
// `unset "a[$k]"` leaves every one of them, which is the row
// interp.Runner.subscriptOperandRead already carried.
func TestUnsetsLexedSubscriptIsTheKeyItExpandedTo(t *testing.T) {
	const table = `declare -A a; a['x]y']=1; a[']']=2; a['[']=3; a['\']=4; a[x]=5; a["'q'"]=6; `
	for _, tc := range []struct{ name, key, gone string }{
		{"a key holding a closing bracket", `x]y`, `x]y`},
		{"a key that is a closing bracket", `]`, `]`},
		{"a key that is an opening bracket", `[`, `[`},
		{"a key that is a backslash", `\`, `\`},
		{"a key with nothing to quote", `x`, `x`},
		{"a key made of quote characters", `'q'`, `'q'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := table + `k='` + quoteForSingle(tc.key) + `'; unset a[$k]; printf "[%s][%s]" "${#a[@]}" "${a[$k]+still}"`
			out, st := answersRun(t, src)
			if want := "[5][]"; out != want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// And the quoted spelling still removes nothing, which is what says the rule
// is about the brackets the source wrote rather than about the key.
func TestUnsetsQuotedOperandStillFindsNoBracketedKey(t *testing.T) {
	const table = `declare -A a; a[']']=2; a[x]=5; `
	out, st := answersRun(t, table+`k=']'; unset "a[$k]"; printf "[%s]" "${#a[@]}"`)
	if want := "[2]"; out != want || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, want)
	}
}

// quoteForSingle puts a key inside a single-quoted word of the source above.
// Only the apostrophe needs the dance, and the keys here hold at most one.
func quoteForSingle(s string) string {
	out := ""
	for _, c := range s {
		if c == '\'' {
			out += `'\''`
			continue
		}
		out += string(c)
	}
	return out
}

// One written pair, and the rule is about the pairs the **source** wrote
// rather than about the brackets in the text: `unset a[1][2]` wrote two, so
// the operand is not one subscript and is read as the name it is not.
// Measured 2026-09-22 on bash 5.3.20, `a=(x y z); unset a[1][2]` is silent
// at status 0 with all three elements still there.
//
// A lexed pair may hold the other bracket and still be one pair, which is
// the row above this one — so the two cannot be told apart by counting the
// brackets in the expanded text, only by what the source wrote.
func TestUnsetDoesNotReadAChainedSubscriptAsOne(t *testing.T) {
	out, st := answersRun(t, `a=(x y z); unset a[1][2]; printf "[%s][%s]" "${#a[@]}" "${a[1]}"`)
	if want := "[3][y]"; !strings.Contains(out, want) {
		t.Errorf("= %q status %d, want it to contain %q with the array untouched", out, st, want)
	}
}
