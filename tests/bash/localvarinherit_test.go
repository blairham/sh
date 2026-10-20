// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `shopt -s localvar_inherit` makes a valueless local declaration take the
// value **and the attributes** of the name at the enclosing scope instead of
// starting empty, and `local -I` asks for the same thing one declaration at a
// time. They are one request with one answer.
//
// Measured 2026-09-17 on bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// from a script file, over a caller's `v=OUTER`, `declare -i n=5`,
// `declare -a a=(x y)` and `declare -A m=([k]=w)`.
//
// Both were refused before #3434 — the name as `localvar_inherit: not
// implemented` and the letter as `-I is not implemented yet` — and the
// refusal was the honest kind, since the whole observable is what the local
// holds: a shell that granted the name and went on making empty bindings
// would be the silent wrong answer.
const localInheritOuter = `v=OUTER; declare -i n=5; declare -a a=(x y); declare -A m=([k]=w); `

func TestLocalVarInheritTakesTheEnclosingValueAndItsAttributes(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The default, which is the fresh binding every shell with a local
		// scope makes.
		{
			localInheritOuter + `f(){ local v n a m; declare -p v n a m; }; f`,
			"declare -- v\ndeclare -- n\ndeclare -- a\ndeclare -- m\n",
		},
		// And the option, where the attributes travel with the value.
		{
			`shopt -s localvar_inherit; ` + localInheritOuter +
				`f(){ local v n a m; declare -p v n a m; }; f`,
			"declare -- v=\"OUTER\"\ndeclare -i n=\"5\"\n" +
				"declare -a a=([0]=\"x\" [1]=\"y\")\ndeclare -A m=([k]=\"w\" )\n",
		},
		// The letter is the same request with the option off.
		{localInheritOuter + `f(){ local -I v; declare -p v; }; f`, "declare -- v=\"OUTER\"\n"},
		{localInheritOuter + `f(){ local -I n; declare -p n; }; f`, "declare -i n=\"5\"\n"},
		{
			localInheritOuter + `f(){ local -I a; declare -p a; }; f`,
			"declare -a a=([0]=\"x\" [1]=\"y\")\n",
		},
		// `+I` is the same request too, which is measured rather than
		// assumed: there is no spelling that turns inheritance off for one
		// declaration.
		{localInheritOuter + `f(){ local +I v; declare -p v; }; f`, "declare -- v=\"OUTER\"\n"},
		// Under the other two spellings of the word.
		{localInheritOuter + `f(){ declare -I v; declare -p v; }; f`, "declare -- v=\"OUTER\"\n"},
		{localInheritOuter + `f(){ typeset -I v; declare -p v; }; f`, "declare -- v=\"OUTER\"\n"},
		// A value on the declaration wins, and nothing is inherited beside
		// it.
		{localInheritOuter + `f(){ local -I v=NEW; declare -p v; }; f`, "declare -- v=\"NEW\"\n"},
		// The declaration's own letters are kept and the inherited ones are
		// added to them — the scalar becoming the array's first element,
		// which is what a cell that is *not* being built empty does.
		{localInheritOuter + `f(){ local -I -i v; declare -p v; }; f`, "declare -i v=\"OUTER\"\n"},
		{localInheritOuter + `f(){ local -I -a n; declare -p n; }; f`, "declare -ai n=([0]=\"5\")\n"},
		// A name the enclosing scope does not hold is inherited as nothing
		// rather than as the empty string, and the declaration still
		// succeeds.
		{`shopt -s localvar_inherit; f(){ local zzz; declare -p zzz; echo "st=$?"; }; f`, "declare -- zzz\nst=0\n"},
		// Only a fresh cell: a second declaration of a name its own scope
		// already made has no enclosing binding in front of it.
		{
			`shopt -s localvar_inherit; v=O; f(){ local v=S; local v; declare -p v; }; f`,
			"declare -- v=\"S\"\n",
		},
		// The *enclosing* scope and not the global, which is what makes this
		// a scope question rather than a lookup.
		{
			`shopt -s localvar_inherit; v=OUT; o(){ local v=MID; i; }; i(){ local v; declare -p v; }; o`,
			"declare -- v=\"MID\"\n",
		},
		// And the way back.
		{
			`shopt -s localvar_inherit; shopt -u localvar_inherit; v=O; f(){ local v; declare -p v; }; f`,
			"declare -- v\n",
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The caller's name is untouched on return, which is the control every row
// above leans on: this is a local that starts from a copy, not a declaration
// that reaches the enclosing scope.
func TestLocalVarInheritLeavesTheEnclosingNameAlone(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`shopt -s localvar_inherit; v=O; f(){ local v; v=NEW; declare -p v; }; f; declare -p v`,
			"declare -- v=\"NEW\"\ndeclare -- v=\"O\"\n",
		},
		{
			`shopt -s localvar_inherit; declare -a a=(x y); f(){ local a; a+=(z); declare -p a; }; f; declare -p a`,
			"declare -a a=([0]=\"x\" [1]=\"y\" [2]=\"z\")\ndeclare -a a=([0]=\"x\" [1]=\"y\")\n",
		},
		{
			`v=O; f(){ local -I v; v=NEW; }; f; declare -p v`,
			"declare -- v=\"O\"\n",
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// Three edges the measurement fixed, each of which a smaller change would
// have got wrong.
func TestLocalVarInheritEdges(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		// The name-reference attribute does not travel: the *text* the
		// reference held is inherited and the reference is not. A local that
		// inherited the reference would aim the callee's own name at whatever
		// the caller was pointing at.
		{
			`shopt -s localvar_inherit; declare -n nr=v; v=O; f(){ local nr; declare -p nr; }; f`,
			"declare -- nr=\"v\"\n", 0,
		},
		// The inherited compound is what a container letter on the same line
		// then has to convert, so the refusal that a fresh empty cell never
		// meets is reached here.
		{
			`shopt -s localvar_inherit; declare -a a=(x y); f(){ local -A a; declare -p a; }; f`,
			"bash: line 1: local: a: cannot convert indexed to associative array\n" +
				"declare -a a=([0]=\"x\" [1]=\"y\")\n", 0,
		},
		// The letter says nothing about what a *name* is, so it neither
		// filters a listing nor suppresses the bare record a valueless
		// declaration makes.
		{`f(){ local -I zz; declare -p zz; }; f`, "declare -- zz\n", 0},
		// An exported name is inherited exported, far enough to reach a
		// child.
		{
			`shopt -s localvar_inherit; declare -x e=EXP; f(){ declare -p e; }; f`,
			"declare -x e=\"EXP\"\n", 0,
		},
		// And the name is answered by the builtin the way every wired name
		// is, which it was not while it sat in the refusing table.
		{`shopt -s localvar_inherit; shopt -p localvar_inherit`, "shopt -s localvar_inherit\n", 0},
		{`shopt -p localvar_inherit`, "shopt -u localvar_inherit\n", 1},
		{`shopt -s localvar_inherit; echo "st=$?"`, "st=0\n", 0},
		{
			`shopt -s localvar_inherit; case ":$BASHOPTS:" in *:localvar_inherit:*) echo in ;; *) echo out ;; esac`,
			"in\n", 0,
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}
