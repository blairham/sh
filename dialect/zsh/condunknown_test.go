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

// A `-word` this shell has **no condition for at all** is the same reading:
// parsed here, and refused by name when it runs.
//
// The half #965 left and #4261 took. `-Q` is the control rather than an
// operator anybody wants — nothing implements it — so these rows are about
// the *spelling* and not about one missing letter, which is why `-R` sits
// beside it rather than standing alone.
//
// Measured on zsh 5.9.2, 2026-09-22, over `-c` with `LC_ALL=C`.
func TestAnUnknownConditionIsRefusedWhenTheConditionRuns(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"the issue's own line", `echo pre; [[ -Q x ]]; echo post`,
			"pre\nzsh:1: unknown condition: -Q\n", 2,
		},
		{
			"a letter another shell has", `echo pre; [[ -R x ]]; echo post`,
			"pre\nzsh:1: unknown condition: -R\n", 2,
		},
		{
			"a name rather than a letter", `echo pre; [[ -bogus x ]]; echo post`,
			"pre\nzsh:1: unknown condition: -bogus\n", 2,
		},
		{
			"a two-operand operator in front", `echo pre; [[ -eq x ]]; echo post`,
			"pre\nzsh:1: unknown condition: -eq\n", 2,
		},
		{
			"with no operand at all", `echo pre; [[ -Q ]]; echo post`,
			"pre\nzsh:1: unknown condition: -Q\n", 2,
		},
		{
			"with two operands", `echo pre; [[ -Q x y ]]; echo post`,
			"pre\nzsh:1: unknown condition: -Q\n", 2,
		},
		// The refusal reaches out of a negation and out of a group, as the
		// arity one does.
		{
			"inside a negation", `echo pre; [[ ! -Q x ]]; echo post`,
			"pre\nzsh:1: unknown condition: -Q\n", 2,
		},
		{
			"inside a group", `echo pre; [[ ( -Q ) ]]; echo post`,
			"pre\nzsh:1: unknown condition: -Q\n", 2,
		},
		// **The row that says this is a lookup and not a wording.** The
		// short-circuit reaches the `]]` with the operator never resolved,
		// so there is nothing to complain about — which a parse-time refusal
		// cannot do however it is worded.
		{
			"never reached, never refused", `echo pre; [[ 1 == 1 || -Q x ]]; echo st=$?`,
			"pre\nst=0\n", 0,
		},
		{
			"nor on the unevaluated side of an &&", `echo pre; [[ 1 == 2 && -Q x ]]; echo st=$?`,
			"pre\nst=1\n", 0,
		},
		// The boundaries. A name of three characters or more with nothing
		// behind it is an ordinary word, where a two-character one is an
		// operator wherever it stands — the rows above have `[[ -Q ]]`.
		{"a long name alone", `echo pre; [[ -zz ]]; echo st=$?`, "pre\nst=0\n", 0},
		{"a long name before a connective", `echo pre; [[ -zz && -n x ]]; echo st=$?`, "pre\nst=0\n", 0},
		// And a two-operand operator behind the word makes the word its left
		// operand, which is what keeps a negative number out of the refusal.
		{"a comparison's left operand", `echo pre; [[ -Q == bar ]]; echo st=$?`, "pre\nst=1\n", 0},
		{"a negative number", `echo pre; [[ -1 -lt 2 ]]; echo st=$?`, "pre\nst=0\n", 0},
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

// `-a` is `-e` under its older spelling, and it is here because #4261 is what
// made its absence a *sentence*: with every unknown `-word` refused by name,
// a missing operator every column has would have this shell saying `unknown
// condition: -a` about a file test.
//
// Measured 2026-09-22: `[[ -a /etc ]]` is 0 and `[[ -a /nosuch ]]` is 1 on
// bash 5.3, bash 3.2, ksh93u+ and zsh 5.9.2 alike. The third row is the one
// that says the word is not a connective inside `[[ ]]`, which is the trap:
// in the `[` builtin it is `and`.
func TestTheOlderSpellingOfTheExistenceTest(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, "[[ -a "+dir+" ]]; echo dir=$?\n"+
		"[[ -a "+dir+"/nosuch ]]; echo miss=$?\n"+
		"[ x -a y ]; echo builtin=$?\n")
	if want := "dir=0\nmiss=1\nbuiltin=0\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
