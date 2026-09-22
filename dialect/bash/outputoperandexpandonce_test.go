// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// With `shopt -s assoc_expand_once` there is no second round over an output
// operand's subscript, so `printf -v a[$k]` and `read a[$k]` take what the
// brackets expanded to as the key — a quote character in it, a bracket in it.
//
// Without the option the same operands are bad names, and that is the same
// fact rather than a second one: the refusal is what the round runs into. A
// text like `a[80's]` re-expanded meets a quote that never closes.
//
// Measured 2026-09-22 against bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with a table declared in front of each row:
//
//	          shopt -s assoc_expand_once      without it
//	k=']'     printf -v A[$k] r → key `]`     `printf: A[]]': not a valid identifier`
//	k='['     read A[$k] → key `[`            `read: A[[]': not a valid identifier`
//	k="80's"  printf -v A[$k] v → key `80's`  the same refusal
//	k=']'     declare A[$k]=Z → refused       refused
//
// The declaration is the control and it is the row that says this belongs to
// the output operand rather than to operands in general: its subscript rounds
// under an axis of its own and bash's option does not move it, which
// interp.Runner.ExpandsAnOperandsSubscriptAgain already records.
func TestAnOutputOperandsLexedSubscriptIsTheKeyWhenTheRoundIsOff(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"printf -v through a closing bracket",
			`shopt -s assoc_expand_once; declare -A A; k=']'; printf -v A[$k] r; printf "[%s][%s]" "$?" "${A[$k]}"`,
			`[0][r]`,
		},
		{
			"read through an opening bracket",
			`shopt -s assoc_expand_once; declare -A A; k='['; read A[$k] <<< l; printf "[%s][%s]" "$?" "${A[$k]}"`,
			`[0][l]`,
		},
		{
			"printf -v through a key holding an apostrophe",
			`shopt -s assoc_expand_once; declare -A A; k="80's"; printf -v A[$k] v; printf "[%s][%s]" "$?" "${A[$k]}"`,
			`[0][v]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := answersRun(t, tc.src); out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And with the round on — the default — every one of them is the bad name it
// has always been here, which is what keeps the change to the one state the
// option names.
func TestAnOutputOperandsLexedSubscriptIsStillABadNameWithTheRoundOn(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"printf -v", `declare -A A; k=']'; printf -v A[$k] r`},
		{"read", `declare -A A; k='['; read A[$k] <<< l`},
		{
			"a declaration, which the option does not move either",
			`shopt -s assoc_expand_once; declare -A A; k=']'; declare A[$k]=Z`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if st == 0 {
				t.Errorf("= %q status %d, want a refusal", out, st)
			}
		})
	}
}
