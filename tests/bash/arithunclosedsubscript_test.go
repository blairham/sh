// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A name whose subscript never closes is a **bad subscript** here rather than
// text an expression could not use, and the refusal names it from the name.
//
// Measured 2026-09-23 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C bash f.sh`, standard input on the null device, bash 5.3.20:
//
//	let 'b[c'        `b[c: bad array subscript (error token is "b[c")`
//	let 'b['         the same, token `b[`
//	let 'a[1]+b[c'   the same, token `b[c` — the name is backed over
//	let 'b[c&'       the same, token `b[c&`
//	let 'a[1] b[c'   the same, token `b[c`
//	let 'b[c[d'      the same, token `b[c[d`
//	$(( b[c ))       the same, token `b[c ` — to the end, blank included
//
// This shell blamed the bracket as a leftover operator: `[c`, under
// `invalid arithmetic operator`. ksh93 keeps its own generic sentence, which
// names no token, and zsh does not refuse the shape at all — so the reason is
// bash's and a dialect that says nothing is unchanged (#4175).
func TestAnUnclosedArithmeticSubscriptIsABadSubscriptHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare name", `let 'b[c'`, `b[c: bad array subscript (error token is "b[c")`},
		{"nothing in the brackets", `let 'b['`, `b[: bad array subscript (error token is "b[")`},
		{
			"behind an operator, the name backed over",
			`let 'a[1]+b[c'`,
			`a[1]+b[c: bad array subscript (error token is "b[c")`,
		},
		{
			"with a byte behind it",
			`let 'b[c&'`,
			`b[c&: bad array subscript (error token is "b[c&")`,
		},
		{
			"behind a complete subscript",
			`let 'a[1] b[c'`,
			`a[1] b[c: bad array subscript (error token is "b[c")`,
		},
		{
			"nested and unclosed",
			`let 'b[c[d'`,
			`b[c[d: bad array subscript (error token is "b[c[d")`,
		},
		{
			"inside an arithmetic expansion, to the end",
			`echo $(( b[c ))`,
			`b[c : bad array subscript (error token is "b[c ")`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want %q in it", out, tc.want)
			}
		})
	}
}

// The controls, and they are what keep the extent honest: a bracket that closes,
// and one with no name in front of it, stay on the ordinary leftover sentences.
//
// Same run:
//
//	let 'b[c]['               `invalid arithmetic operator`, token `[`
//	let '[c'                  `operand expected`, token `[c`
//	let 'b[c] + 1'            no complaint at all
//	a=(1 2); k='x],b[$(cmd)'; ${a[$k]}   `invalid arithmetic operator`, token
//	                 `],b[$(cmd)` — the leftover opens at the `]` after `x`, so
//	                 the reader stopped there and the `b[` further along is not
//	                 what it refused. This is the row an earlier draft broke.
func TestAClosedOrNamelessBracketIsNotABadSubscriptHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a stray bracket behind a closed one",
			`let 'b[c]['`,
			`b[c][: arithmetic syntax error: invalid arithmetic operator (error token is "[")`,
		},
		{
			"a bracket with no name in front of it",
			`let '[c'`,
			`[c: arithmetic syntax error: operand expected (error token is "[c")`,
		},
		{
			// The two rows that prove the bracket has to follow the name
			// **immediately**: the reader stopped at a byte it refuses, a name
			// with an unclosed bracket stands behind that byte, and this column
			// still says `invalid arithmetic operator` about the whole leftover.
			// Without the immediacy test both come back as bad subscripts named
			// from `b`.
			"a refused byte with an unclosed subscript behind it",
			`let 'b@c[d'`,
			`b@c[d: arithmetic syntax error: invalid arithmetic operator (error token is "@c[d")`,
		},
		{
			"and the same with a backtick",
			"let 'b`c[d'",
			"b`c[d: arithmetic syntax error: invalid arithmetic operator (error token is \"`c[d\")",
		},
		{
			// A name the reader *did* stop right at still earns the subscript
			// sentence, which is the pair to the two rows above.
			"a name the reader stopped at keeps the subscript sentence",
			`let 'b&c[d'`,
			`b&c[d: bad array subscript (error token is "c[d")`,
		},
		{
			// The shape that parts the two readings, and the one an earlier
			// draft of this broke: the leftover opens at the `]` after `x`, so
			// the reader stopped there and the `b[` further along is not what it
			// was refusing.
			"a subscript whose own text the reader gave up on earlier",
			`declare -a a=(1 2); k='x],b[$(echo hi)'; echo "${a[$k]}"`,
			`x],b[$(echo hi): arithmetic syntax error: invalid arithmetic operator (error token is "],b[$(echo hi)")`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want %q in it", out, tc.want)
			}
		})
	}
}

// A closed subscript is no complaint at all, which is the control that says the
// scan is not simply firing on every bracket.
func TestAClosedArithmeticSubscriptIsStillFine(t *testing.T) {
	out, st := answersRun(t, `b=(0 7); let 'b[1] + 1'; printf '[%s]' "$?"`)
	if out != "[0]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[0]")
	}
}
