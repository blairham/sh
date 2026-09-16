// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A loop variable that is a positional parameter's number (#3040).
//
// `for 1 in a b` sets `$1` on each pass. One shell in the panel has it and
// seven of the completion functions it ships are written with it, so a parser
// without it cannot read them at all.
//
// Measured 2026-09-15, each probe in a script file of its own:
//
//	| probe                  | zsh 5.9.2   | bash 5.3 | bash-as-sh | bash 3.2 | ksh93u+ | dash | ash |
//	| `for 1 in a b`         | `a` `b`     | refused  | refused    | refused  | refused | refused | refused |
//	| `for 1 2 in a b c d`   | `a-b` `c-d` | refused  | refused    | refused  | refused | refused | refused |
//	| `set -- p q r; for 1;` | `p` `q` `r` | refused  | refused    | refused  | refused | refused | refused |
//
// The three bash columns say `` `1': not a valid identifier ``, ksh93
// `invalid variable name`, dash and BusyBox ash `bad for loop variable` — so
// one column against six and this is a dialect's grammar.
//
// **Digits and nothing else.** `for 0`, `for 12` and `for 01` are all taken
// in the accepting column and `for 1x` and `for @` are refused there, so the
// flag admits a positional parameter's number rather than relaxing what a
// name is.

func forPositionalNameGrammar(d *Dialect) {
	d.ForNameMayBeAPositionalParameter = true
	// One row below names two variables at once, which is a flag of its own.
	d.ForMultipleNames = true
}

// forNames is the loop-variable list of the first `for` in src.
func forNames(t *testing.T, src string, d Dialect) []string {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*ForClause)
	if !ok {
		t.Fatalf("%s: first command is %T, want a for clause", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	if c.RefusedName != "" {
		t.Fatalf("%s: the name %q was carried to the loop rather than taken", src, c.RefusedName)
	}
	return c.Names
}

func TestALoopVariableMayBeAPositionalParametersNumber(t *testing.T) {
	t.Parallel()
	d := Core()
	forPositionalNameGrammar(&d)
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"one digit", "for 1 in a b; do :; done", []string{"1"}},
		{"more than one", "for 12 in a; do :; done", []string{"12"}},
		{"a leading zero", "for 01 in a; do :; done", []string{"01"}},
		{"nought", "for 0 in a; do :; done", []string{"0"}},
		{"two of them", "for 1 2 in a b c d; do :; done", []string{"1", "2"}},
		{"with no word list", "for 1; do :; done", []string{"1"}},
		{"beside an ordinary name", "for 1 x in a b; do :; done", []string{"1", "x"}},
		{
			// The control that says nothing moved for ordinary loops.
			"an ordinary name on its own",
			"for i in a b; do :; done",
			[]string{"i"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := forNames(t, tc.src, d)
			if len(got) != len(tc.want) {
				t.Fatalf("%s: %d names %q, want %d %q", tc.src, len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("%s: name %d is %q, want %q", tc.src, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestALoopVariableOfDigitsIsDigitsAndNothingElse is what keeps the flag from
// reading as "the name check is off": the accepting column refuses a word that
// is digits and then something else, and refuses a special parameter's name
// outright.
func TestALoopVariableOfDigitsIsDigitsAndNothingElse(t *testing.T) {
	t.Parallel()
	d := Core()
	forPositionalNameGrammar(&d)
	for _, src := range []string{
		"for 1x in a; do :; done",
		"for @ in a; do :; done",
		"for 1.2 in a; do :; done",
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%s parsed, and the shell this flag is read from refuses it", src)
		}
	}
}

// TestALoopVariableOfDigitsIsOneDialectsAndNotEveryShells is the panel's other
// side: six columns refuse the header, so the core grammar must too.
func TestALoopVariableOfDigitsIsOneDialectsAndNotEveryShells(t *testing.T) {
	t.Parallel()
	d := Core()
	if _, err := Parse("for 1 in a b; do :; done", d); err == nil {
		t.Error("`for 1 in a b` parsed without the flag that reads a number as a loop variable")
	}
}
