// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `set -A` re-creates the name it writes on exactly the rows `a=(…)` does —
// #4768, and the store that reached the array table without ever asking.
//
// Measured 2026-09-27 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), run `-f` from a script file under `env -i
// PATH=/usr/bin:/bin`; `go version -m` says *not a Go executable* for it.
//
// The grid varies the two things the rule is keyed on — the **form** (`-A`
// against `+A`) and what the name **was** (a scalar, a declared array holding
// nothing, an array holding elements) — because the form alone parts this
// column from ksh93 on two of the rows and the prior shape parts it from
// itself on two more. An attribute that merely moved with the form would
// agree with a grid that varied only the letter.
func TestSetArrayDropsTheAttributesAReCreationDrops(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the minus form over a scalar re-creates",
			`export a=1; set -A a x y; typeset -p a`,
			"typeset -a a=( x y )\n",
		},
		{
			"and over an array it is holding, it does not",
			`export a=(p q); set -A a x y; typeset -p a`,
			"typeset -ax a=( x y )\n",
		},
		{
			"nor over a declared array holding nothing",
			`typeset -a a; export a; set -A a x; typeset -p a`,
			"typeset -ax a=( x )\n",
		},
		{
			"the plus form over a scalar re-creates",
			`export a=1; set +A a x y; typeset -p a`,
			"typeset -a a=( x y )\n",
		},
		{
			"and over an array it is holding, it does not",
			`export a=(p q); set +A a x y; typeset -p a`,
			"typeset -ax a=( x y )\n",
		},
		// The type letters go with the export attribute rather than beside
		// it, which is what says this is the re-creation question and not an
		// export rule — and the value follows, because the letter is off the
		// name before the elements land.
		{
			"the integer letter goes, so nothing is folded",
			`typeset -i b=1; set -A b 5+5; typeset -p b`,
			"typeset -a b=( 5+5 )\n",
		},
		{
			"a case letter goes on a scalar",
			`typeset -l c=AB; set -A c CD; typeset -p c`,
			"typeset -a c=( CD )\n",
		},
		{
			"and stays on an array it is holding",
			`typeset -la c=(AB); set -A c CD; typeset -p c`,
			"typeset -al c=( CD )\n",
		},
		// A store that does not happen takes nothing off the name: `set +A`
		// with no values leaves the array exactly as it was.
		{
			"a plus form with no values changes nothing",
			`export e=(p q); set +A e; typeset -p e`,
			"typeset -ax e=( p q )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The control beside it: the literal spellings of the same three shapes,
// which #4676 already made right and which this must not move. They are the
// rows the axes were measured on, so a change that answered `set -A` by
// widening the clearing would show up here.
func TestTheLiteralRoutesAreUnmovedBySetArray(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a literal over an exported scalar", `export a=1; a=(x y); typeset -p a`, "typeset -a a=( x y )\n"},
		{"an append over an exported scalar", `export a=1; a+=(x y); typeset -p a`, "typeset -a a=( 1 x y )\n"},
		{"a literal over an exported array", `export a=(p q); a=(x y); typeset -p a`, "typeset -ax a=( x y )\n"},
		{"a literal over a declared empty array", `typeset -a a; export a; a=(x); typeset -p a`, "typeset -ax a=( x )\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
