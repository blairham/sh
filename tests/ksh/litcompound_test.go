// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A compound variable's body written **inside an array literal** —
// `a=([1]=(p=1 q=2))` and `a=(x (p=1 q=2))` — which is the same construct
// #2853 landed for `a[1]=(p=1 q=2)`, reached through a second parse site and a
// second store site.
//
// Every row was measured on ksh93u+ 2012-08-01, 2026-09-20, each one its own
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C /bin/ksh x.sh` with
// standard input on `/dev/null` and the output read through `sed -n l`. The
// rows are written out in docs/spec/semantics.md, "A compound body inside an
// array literal" (#3864).
func TestACompoundBodyInsideAnArrayLiteral(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The issue's three rows. The shape already agreed — the name is a
		// table and the body sits at the subscript — so what moves is the
		// nested value's reading, and the `;` that says it is a body.
		{`a=([1]=(p=1 q=2)); typeset -p a`, "typeset -A a=([1]=(p=1;q=2))\n"},
		{`a=(x (p=1 q=2)); typeset -p a`, "typeset -A a=([0]=x [1]=(p=1;q=2))\n"},
		// Read from the other end: a `;`-joined body was a syntax error
		// wherever it stood inside a literal, so this shell could not parse
		// back what it would print.
		{`typeset -a c=([0]=(a=1;b=2)); typeset -p c`, "typeset -a c=((a=1;b=2))\n"},
		// The same first word decides it that decides over a bare name, so a
		// literal opening with `x` is still the nested array — the control,
		// and it does not move.
		{`d=([1]=(x y)); typeset -p d`, "typeset -A d=([1]=(x y) )\n"},
		{`m=(x (y z)); typeset -p m`, "typeset -A m=([0]=x [1]=(y z) )\n"},
		// And a *quoted* assignment is not one, which is what says the
		// reading is of what was written: the element is the nested array
		// holding the one string. Read rather than listed, because how that
		// string's own `=` is *written* back is #3863 and not this.
		{`a=( ("a=1") ); printf '[%s]' "${a[0][0]}" "${#a[0][@]}"`, "[a=1][1]"},
		// The members are reachable, by every spelling the subscripted
		// assignment's are.
		{`f=([1]=(p=1 q=2)); printf '[%s]' "${f[1].p}" "${f[1].q}"`, "[1][2]"},
		{`g=(x (p=1 q=2)); printf '[%s]' "${g[1].q}"`, "[2]"},
		{`f=([1]=(p=1 q=2)); typeset -p 'f[1]'`, "typeset -C f[1]=(p=1;q=2)\n"},
		{`a=( (p=1) ); print -r -- ${!a[0].@}`, "a[0].p\n"},
		{`i=([1]=(p=1 q=2)); printf '[%s]' "${i[1]}"`, "[(\n\tp=1\n\tq=2\n)]"},
		// A member is written after the fact, and the element counts as one.
		{`a=( (p=1) ); a[0].q=7; typeset -p a`, "typeset -a a=((p=1;q=7))\n"},
		{`a=( (p=1) ); echo "${#a[@]} ${!a[@]}"`, "1 0\n"},
		// A body may nest and may carry a declarator, which is the whole of
		// the body grammar reaching this site.
		{
			`a=( (p=1 q=(r=2)) ); typeset -p a; printf '[%s]' "${a[0].q.r}"`,
			"typeset -a a=((p=1;q=(r=2)))\n[2]",
		},
		{
			`a=( (typeset -i n=5) ); typeset -p a; printf '[%s]' "${a[0].n}"`,
			"typeset -a a=((typeset -i n=5))\n[5]",
		},
		// An **empty** pair of parentheses is the compound here too, and the
		// first row is where that is invisible: the element's own listing is
		// the same under either reading. The two after it are where it is not.
		{`a=( () ); typeset -p a`, "typeset -a a=(())\n"},
		{`a=( () ); typeset -p 'a[0]'`, "typeset -C a[0]=()\n"},
		{`a=( () ); a[0].p=3; typeset -p a`, "typeset -a a=((p=3))\n"},
		// The retyping rule counts a compound element exactly as it counts a
		// nested array: a first element of its own parentheses keeps the
		// literal indexed, and one that is plain makes it a table.
		{`a=( (p=1) (q=2) ); typeset -p a`, "typeset -a a=((p=1) (q=2))\n"},
		{`a=( () x ); typeset -p a`, "typeset -a a=(() x)\n"},
		{`a=(x (p=1 q=2) y); typeset -p a`, "typeset -A a=([0]=x [1]=(p=1;q=2) [2]=y)\n"},
		{`a=([0]=(p=1) [1]=(q=2)); typeset -p a`, "typeset -A a=([0]=(p=1) [1]=(q=2))\n"},
		// The element loses its members with its value, by the routes the
		// subscripted spelling's does.
		{`a=( (p=1) ); a[0]=(x y); typeset -p a`, "typeset -a a=((x y) )\n"},
		{`a=( (p=1) ); unset a; printf '[%s]' "${a[0].p}"`, "[]"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}
