// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `<(` and `>(` inside brackets the script wrote as an array subscript are the
// arithmetic `<` and `>` with a grouping behind them, not a process
// substitution.
//
// Measured 2026-09-22 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C bash f.sh`, standard input on the null device, bash 5.3.20, with
// `a=(1 2 3)`:
//
//	${a[0<(1)]}            2 — the subscript is `0<1`, which is 1
//	${a[1>(2)]}            1 — and `1>2`, which is 0
//	${a[ 1 <(2) ]}         2 — blanks change nothing
//	A[1<(2)]=v             the key is the five characters `1<(2)`
//	c[1<(2)]=9             element 1, the subscript being `1<2`
//	[[ a[1<(2)] -lt 9 ]]   holds, and nothing is run
//
// Performing the substitution answered a script from a descriptor number
// divided into a subscript: `${a[1<(2)]}` was `1/dev/fd/63: division by 0` and
// handed back nothing where the element is `2`, with a diagnostic naming a path
// no line of the script mentions (#4253).
func TestAProcSubstInAWrittenSubscriptIsArithmetic(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an expansion's subscript",
			`a=(1 2 3); printf '[%s]' "${a[0<(1)]}"`,
			"[2]",
		},
		{
			"the output spelling in one",
			`a=(1 2 3); printf '[%s]' "${a[1>(2)]}"`,
			"[1]",
		},
		{
			"blanks around it change nothing",
			`a=(1 2 3); printf '[%s]' "${a[ 1 <(2) ]}"`,
			"[2]",
		},
		{
			"a keyed table's key is the characters",
			`declare -A A; A[1<(2)]=v; printf '[%s]' "${A['1<(2)']}"`,
			"[v]",
		},
		{
			"an assignment target's subscript",
			`c=(x y z); c[1<(2)]=9; printf '[%s][%s]' "${c[0]}" "${c[1]}"`,
			"[x][9]",
		},
		{
			"an append to one",
			`d=(x y z); d[1<(2)]+=Q; printf '[%s]' "${d[1]}"`,
			"[yQ]",
		},
		{
			"an arithmetic comparison's operand",
			`a=(1 2 3); [[ a[1<(2)] -lt 9 ]] && printf '[yes]' || printf '[no]'`,
			"[yes]",
		},
		{
			"and the comparison it answers",
			`a=(1 2 3); [[ a[1<(2)] -eq 1 ]] && printf '[yes]' || printf '[no]'`,
			"[no]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The controls, in both directions: the line is the **subscript** and not the
// construct, not `[[ ]]`, and not the brackets on their own.
//
// Same run:
//
//	[[ -e <(echo x) ]]        a substitution, on the same operand route
//	echo a[1<(2)]             `a[1/dev/fd/63]` — brackets in an ordinary
//	                          command word are no subscript in either shell
//	x=a[1<(2)]                the same, an assignment's *value* being a word
//	[[ a[1<(2)] = "a[1]" ]]   a substitution: one word, and the operator is
//	                          what decides which reading it gets
func TestAProcSubstOutsideAWrittenSubscriptIsStillOne(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"at the front of a condition's operand",
			`[[ -e <(echo x) ]] && printf '[yes]' || printf '[no]'`,
			"[yes]",
		},
		{
			"brackets in an ordinary command word",
			`printf '[%s]' "$(echo a[1<(2)])"`,
			"[a[1/dev/fd/",
		},
		{
			"brackets in an assignment's value",
			`x=a[1<(2)]; printf '[%s]' "$x"`,
			"[a[1/dev/fd/",
		},
		{
			"the same operand under a string comparison",
			`a=(1 2 3); [[ a[1<(2)] = "a[1]" ]] && printf '[yes]' || printf '[no]'`,
			"[no]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if st != 0 || !strings.HasPrefix(out, tc.want) {
				t.Errorf("= %q status %d, want a prefix of %q at 0", out, st, tc.want)
			}
		})
	}
}
