// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A known conditional operator standing with the wrong number of operands is
// **parsed** here and refused when it runs.
//
// That is the second half of #888 and the whole of #965, and it is a claim
// about *where* the refusal happens rather than about how it is worded: the
// `echo pre` in every row is what tells the two apart. bash and ksh93 name
// the offending token while reading and never run it; this shell runs it and
// then complains about the operator.
//
// Measured on zsh 5.9.2, 2026-09-14 and again 2026-09-15, over `-c` and from
// a script file, with `env -i PATH=/usr/bin:/bin`.
func TestASurplusOperandIsRefusedWhenTheConditionRuns(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		// The three rows #965 tabulates. One sentence for all of them, and
		// it names the *operator* — there is no surplus word in the third.
		{
			"a second operand", `echo pre; [[ -n x y ]]; echo post`,
			"pre\nzsh:1: unknown condition: -n\n", 2,
		},
		{
			"a second operator's worth of words", `echo pre; [[ -n x -z "" ]]; echo post`,
			"pre\nzsh:1: unknown condition: -n\n", 2,
		},
		{
			"no operand at all", `echo pre; [[ -n ]]; echo post`,
			"pre\nzsh:1: unknown condition: -n\n", 2,
		},
		// Another operator, so the row is about the arity rather than about
		// `-n`.
		{
			"a different operator", `echo pre; [[ -o ]]; echo post`,
			"pre\nzsh:1: unknown condition: -o\n", 2,
		},
		// It is fatal, and `post` never running is only half of that: these
		// two say the refusal reaches out of a group and out of a negation
		// rather than being a value either of them could absorb.
		{
			"inside a negation", `echo pre; [[ ! -n ]]; echo post`,
			"pre\nzsh:1: unknown condition: -n\n", 2,
		},
		{
			"inside a group", `echo pre; [[ ( -n ) ]]; echo post`,
			"pre\nzsh:1: unknown condition: -n\n", 2,
		},
		// And on the right of a `&&`, where the left-hand side has already
		// answered true: the refusal is the second operator's and the shell
		// still stops.
		{
			"on the right of an &&", `echo pre; [[ -n x && -z ]]; echo post`,
			"pre\nzsh:1: unknown condition: -z\n", 2,
		},
		// The controls. An operator-shaped *operand* is an ordinary word, so
		// this is a test for non-emptiness and it holds — without this row
		// the rule reads as "a `-` word after an operator is surplus".
		{"an operator-shaped operand", `echo pre; [[ -n -n ]]; echo post`, "pre\npost\n", 0},
		// A word that is no operator at all is a bare-word test, with or
		// without an operand, which is what keeps this rule off `-bogus`.
		{"a word that is not an operator", `echo pre; [[ -bogus ]]; echo post`, "pre\npost\n", 0},
		// And the ordinary arity still runs.
		{"the right number of operands", `echo pre; [[ -n x ]]; echo post`, "pre\npost\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}
