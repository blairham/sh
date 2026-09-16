// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// This shell's two spellings of a **name reference**, which are one command:
// `nameref r=v` and `typeset -n r=v`.
//
// Measured 2026-09-15 against ksh93u+ 2012-08-01, `env -i` with a scratch
// HOME and no startup files. The mechanism is interp/nameref.go and is shared
// with bash; what this file is for is the three places the two shells part —
// the listing's shape, what a cycle does, and the fatality.
func TestBothSpellingsAreOneReference(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"typeset -n reads through", `v=1; typeset -n r=v; echo "[$r]"`, "[1]\n"},
		{"nameref reads through", `v=1; nameref r=v; echo "[$r]"`, "[1]\n"},
		{"typeset -n writes through", `v=1; typeset -n r=v; r=2; echo "[$v]"`, "[2]\n"},
		{"nameref writes through", `v=1; nameref r=v; r=2; echo "[$v]"`, "[2]\n"},
		// The listing is this shell's own shape — bare assignments with the
		// letters in front — and it writes the *reference*, not the target.
		{"a listing writes the reference", `v=1; nameref r=v; typeset -p r`, "typeset -n r=v\n"},
		{"and the target lists as itself", `v=1; nameref r=v; typeset -p v`, "v=1\n"},
		{"a reference with no target lists bare", `nameref r; typeset -p r`, "typeset -n r\n"},
		// Everything the two shells share, in this shell's spelling.
		{"an array is read through", `v=(a b c); nameref r=v; echo "${r[1]} ${r[*]} ${#r[@]}"`, "b a b c 3\n"},
		{"an element is written through", `v=(a b c); nameref r=v; r[1]=Z; echo "${v[*]}"`, "a Z c\n"},
		{"${!r} is the target name", `v=1; nameref r=v; echo "${!r}"`, "v\n"},
		{"arithmetic writes through", `nameref r=v; v=5; (( r++ )); echo "[$v]"`, "[6]\n"},
		{"a function fills in its caller's variable", `f(){ nameref o=$1; o=filled; }; v=; f v; echo "[$v]"`, "[filled]\n"},
		{"unset removes the target", `v=1; nameref r=v; unset r; echo "${v-UNSET}"`, "UNSET\n"},
		{"unset -n removes the reference", `v=1; nameref r=v; unset -n r; echo "${v-UNSET}"`, "1\n"},
		{"a loop re-points it", `v=1; nameref r=v; for r in x y; do :; done; typeset -p r`, "typeset -n r=y\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A cycle is refused **at the declaration** here, however long the loop, and
// the refusal ends the script because `typeset` is one of this shell's
// special builtins. Both halves are where it parts from bash — see
// Semantics.NamerefCycleIsRefused and Semantics.BadNameToDeclarationFatal.
func TestACycleIsRefusedAtTheDeclaration(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			name: "a reference straight to itself", src: `typeset -n r=r; echo after`,
			want: "ksh: typeset: r: invalid self reference\n", status: 1,
		},
		{
			// bash makes this pair and warns at the read; this shell refuses
			// the declaration that closes the loop, and names *that* one.
			name: "a cycle through another reference", src: `typeset -n a=b; typeset -n b=a; echo after`,
			want: "ksh: typeset: b: invalid self reference\n", status: 1,
		},
		{
			name: "a target that is not a name", src: `typeset -n r=1bad; echo after`,
			want: "ksh: typeset: 1bad: invalid variable name\n", status: 1,
		},
		{
			// The second spelling refuses in the first's words, and ends the
			// script the same way.
			name: "and the same under nameref", src: `nameref r=r; echo after`,
			want: "ksh: typeset: r: invalid self reference\n", status: 1,
		},
		{
			// **Inside a function too**, which is where bash parts company:
			// there `local -n r=r` is a warning and a handle on the outer
			// variable, and here it is the same refusal that ends the same
			// script. Measured 2026-09-15 on ksh93u+ 2012-08-01 (#3048).
			name: "a self reference inside a function is refused as well",
			src:  `r=OUTER; f(){ typeset -n r=r; echo "in=[$r]"; r=SET; }; f; echo "after=[$r]"`,
			want: "ksh: typeset: r: invalid self reference\n", status: 1,
		},
		{
			name: "and under nameref inside one",
			src:  `r=OUTER; f(){ nameref r=r; echo "in=[$r]"; }; f; echo after`,
			want: "ksh: typeset: r: invalid self reference\n", status: 1,
		},
		{
			// A bad name under the second spelling is the first's complaint
			// too, which is what says the word is a front rather than a
			// builtin of its own. It ends the script here as every bad name
			// on a declaration does.
			name: "a bad name speaks as typeset", src: `nameref 1x=v; echo after`,
			want: "ksh: typeset: 1x=v: invalid variable name\n", status: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}
