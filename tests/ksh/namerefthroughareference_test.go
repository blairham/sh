// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A value written to a reference whose chain ends at a reference aimed at
// nothing aims that one, exactly as the value written to it directly would,
// and the trace names it. Measured 2026-11-02 against ksh93u+ 2012-08-01
// (`/bin/ksh`), from `-c` under `env -i PATH=/usr/bin:/bin`, byte for byte.
// This shell re-aimed the reference the script wrote and left the one at the
// end aimed at nothing, traced `+ l=y` (#7114).
//
// The minimal pair is the second row: the same chain with its end aimed
// writes through both references. The rows after it are controls that already
// agreed. ksh93 lists `l` itself as `typeset -n l=.deleted` afterwards, which
// is not modeled; see interp.Semantics.ValueThroughAReferenceAimsTheUnaimedEnd.
func TestAValueThroughAReferenceAimsTheUnaimedOneAtTheEndOfTheChain(t *testing.T) {
	t.Parallel()
	const chain = `typeset -n m; typeset -n l=m; `
	for _, c := range []struct{ name, src, want string }{
		{"the row", chain + `l=y; echo st=$?; typeset -p m`, "st=0\ntypeset -n m=y\n"},
		{"the pair: the end aimed", `x=0; typeset -n m=x; typeset -n l=m; l=5; typeset -p x`, "x=5\n"},
		{"traced under the end", `set -x; ` + chain + `l=y`, "+ typeset -n m\n+ l=m\n+ typeset -n l\n+ m=y\n"},
		{"an append", chain + `l+=y; typeset -p m`, "typeset -n m=y\n"},
		{
			"a declaration's operand",
			`set -x; ` + chain + `typeset l=y; typeset -p m`,
			"+ typeset -n m\n+ l=m\n+ typeset -n l\n+ m=y\n+ typeset l\n+ typeset -p m\ntypeset -n m=y\n",
		},
		{"the direct write", `typeset -n m; m=y; typeset -p m`, "typeset -n m=y\n"},
		{"a loop aims its own name", chain + `for l in y; do :; done; typeset -p l m`, "typeset -n l=y\ntypeset -n m\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
	t.Run("a value that is no name ends the shell", func(t *testing.T) {
		t.Parallel()
		const want = "p.sh: line 1: 1: invalid variable name\n"
		if got, st := runKshFile(t, chain+"l=1; echo st=$?\necho next\n"); got != want || st != 1 {
			t.Errorf("%q at %d, want %q at 1", got, st, want)
		}
	})
}

// A bare `typeset -n m` over a name that is already an aimed reference is a
// declaration of the name the reference reaches: the target becomes a reference
// and takes the value it holds as the name to aim at, so a value that is no
// name is refused in the ordinary sentence and ends the shell. Measured
// 2026-11-02 against ksh93u+ 2012-08-01 (`/bin/ksh`), script files and `-c`
// under `env -i PATH=/usr/bin:/bin`, byte for byte. This shell took the line
// as a no-op at 0 and left `m` aimed at `x` (#7115).
//
// The minimal pair is the first two rows: a target holding a name is aimed at
// it, and one holding `0` is refused. ksh93 lists `m` itself as `typeset -n
// m=.deleted` once its target has become a reference, which is not modeled;
// see interp.Semantics.BareReferenceLetterFollowsTheReference.
func TestABareReferenceLetterOverAnAimedReferenceDeclaresItsTarget(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"the pair: the target holds a name", `x=y; typeset -n m=x; typeset -n m; echo st=$?; typeset -p x`, "st=0\ntypeset -n x=y\nnext\n", 0},
		{"the row", `x=0; typeset -n m=x; typeset -n m; echo st=$?`, "p.sh[1]: typeset: 0: invalid variable name\n", 1},
		{"the target empty", `x=; typeset -n m=x; typeset -n m; echo st=$?`, "p.sh[1]: typeset: : invalid variable name\n", 1},
		{
			"the target unset",
			`typeset -n m=x; typeset -n m; echo st=$?; typeset -p m x`,
			"st=0\ntypeset -n m=x\ntypeset -n x\nnext\n", 0,
		},
		{
			"an element reaches its array",
			`a=(p q); typeset -n m='a[1]'; typeset -n m; echo st=$?`,
			"p.sh[1]: typeset: a: reference variable cannot be an array\n", 1,
		},
		{
			"a subshell confines the refusal",
			`x=y; (typeset -n m=x; x=0; typeset -n m); echo sub=$?; typeset -p x`,
			"p.sh[1]: typeset: 0: invalid variable name\nsub=1\nx=y\nnext\n", 0,
		},
		// Controls, which already agreed.
		{"a valued letter re-aims", `x=1; typeset -n m=x; typeset -n m=z; typeset -p m x`, "typeset -n m=z\nx=1\nnext\n", 0},
		{
			"a function's fresh local",
			`x=y; typeset -n m=x; function f { typeset -n m; typeset -p m; }; f; typeset -p m x`,
			"typeset -n m\ntypeset -n m=x\nx=y\nnext\n", 0,
		},
		{"traced as written", `set -x; x=y; typeset -n m=x; typeset -n m`, "+ x=y\n+ m=x\n+ typeset -n m\n+ typeset -n m\n+ echo next\nnext\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, st := runKshFile(t, c.src+"\necho next\n")
			if got != c.want || st != c.status {
				t.Errorf("%q at %d, want %q at %d", got, st, c.want, c.status)
			}
		})
	}
}
