// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The dialect has the grammar, so the real lines that need it run here.
//
// The substrate's tests name the flag; this one names the shell, which is the
// only place that is allowed — and it is worth having, because a flag nothing
// turns on is a construct nobody can write.
func TestTheAlwaysAssignOperatorIsThisDialects(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The construct as the plugin manager's message formatter writes
			// it, reduced to the assignment itself: a hyphenated key in an
			// association, assigned inside an expansion whose value is then
			// thrown away by `:+`. This is the line that printed eighteen
			// `bad math expression: operand expected at ` + "`=rst'" +
			// ` on every startup (#1369).
			"the plugin manager's formatter, at the assignment",
			`typeset -gA ZI; ZI[__last-formatter-code]=; ` +
				`code=rst; ` +
				`printf "[%s]" "${${ZI[__last-formatter-code]::=$code}:+}"; ` +
				`printf "[%s]" "$ZI[__last-formatter-code]"`,
			"[][rst]",
		},
		{
			// The same shape with the fallback the formatter actually
			// writes: the new code when there is one, else the code already
			// stored. Two calls, so the second reads what the first left.
			"and with the formatter's own fallback chain",
			`typeset -gA ZI; ZI[__last-formatter-code]=; ` +
				`for code ( error "" ehi ) { ` +
				`  : "${${ZI[__last-formatter-code]::=${${code:#}:-${ZI[__last-formatter-code]}}}:+}"; ` +
				`  printf "[%s]" "$ZI[__last-formatter-code]"; }`,
			"[error][error][ehi]",
		},
		{
			"an element of this dialect's arrays, whose base is one",
			`a=(1 2 3); printf "[%s]" "${a[2]::=new}"; printf "[%s]" "${a[@]}"`,
			"[new][1][new][3]",
		},
		{
			"a scalar, on all three states of the parameter",
			`unset v; printf "[%s]" "${v::=a}"; v=; printf "[%s]" "${v::=b}"; ` +
				`v=old; printf "[%s]" "${v::=c}"; printf "[%s]" "$v"`,
			"[a][b][c][c]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The substring is untouched in the dialect that gained the operator, which
// is where a widened disambiguation would actually be felt: every one of
// these is a colon this reading had to leave alone.
func TestTheSubstringStillWorksBesideTheAlwaysAssign(t *testing.T) {
	const src = `v=abcdef; printf "[%s][%s][%s][%s][%s]" ` +
		`"${v:2}" "${v:2:2}" "${v: -2}" "${v:-alt}" "${v:=set}"`
	if out, _ := runZsh(t, t.TempDir(), src); out != "[cdef][cd][ef][abcdef][abcdef]" {
		t.Errorf("got %q, want %q", out, "[cdef][cd][ef][abcdef][abcdef]")
	}
}

// A parameter no assignment can name is refused by name and the script ends,
// which is this dialect's own answer — measured on zsh 5.9.2, all five
// spellings, all fatal at status 1.
func TestTheAlwaysAssignRefusalIsThisDialects(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `echo "${#::=new}"; echo AFTER`)
	wantWholeLines(t, out, "zsh:1: not an identifier: #")
	if strings.Contains(out, "AFTER") || st == 0 {
		t.Errorf("got %q (status %d), want the refusal fatal", out, st)
	}
}
