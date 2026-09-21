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
			//
			// The **reason** is the reference letter's own and not the
			// declaration's: `nameref 1x=v` and `typeset -n 1x=v` are both
			// `is not an identifier` in ksh93u+ where the plain `typeset
			// 1x=v` is `invalid variable name`. This row pinned the plain
			// wording until 2026-09-20 — the letter's sentence was recorded
			// beside BuiltinBadName and not modeled, and the pin was this
			// shell's answer rather than the reference's (#3956).
			name: "a bad name speaks as typeset", src: `nameref 1x=v; echo after`,
			want: "ksh: typeset: 1x=v: is not an identifier\n", status: 1,
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

// The other spelling of #3084: the cell a reference lands on holds nothing
// here too, and ksh93 is the column that says so without a function anywhere
// near the line.
//
// Measured 2026-09-15 on ksh93u+ 2012-08-01, `env -i` with a scratch HOME and
// no startup files, every row run against the real shell and this one side by
// side. Where the scope rows differ from bash's it is the axis this dialect
// already carries — `typeset` declares a local only inside a `function`-word
// function — and not this rule, which is the same in both shells.
func TestAReferenceEmptiesTheCellHereToo(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"at the top level", `r=OUTER; v=VEE; typeset -n r=v; unset -n r; echo "[${r-GONE}]"`,
			"[GONE]\n",
		},
		{
			// A keyword-defined function, which is the only one that gets a
			// scope here: the cell is the local's, so the caller's value is
			// back after the return.
			"in a keyword function the local cell is the empty one",
			`r=OUTER; v=VEE; function f { typeset -n r=v; unset -n r; echo "in=[${r-GONE}]"; }; ` +
				`f; echo "after=[${r-GONE}]"`,
			"in=[GONE]\nafter=[OUTER]\n",
		},
		{
			// And in a POSIX-style one there is no scope, so the cell
			// emptied is the global and the value does not come back. The
			// row that pins the rule to the `n` letter rather than to the
			// local: bash restores OUTER on the same source.
			"and in a POSIX function it is the global cell",
			`r=OUTER; v=VEE; f(){ typeset -n r=v; unset -n r; echo "in=[${r-GONE}]"; }; ` +
				`f; echo "after=[${r-GONE}]"`,
			"in=[GONE]\nafter=[GONE]\n",
		},
		{
			// The second spelling of the declaration reaches the same rule,
			// which is what says the word is a front and not a builtin of
			// its own.
			"nameref empties it as well",
			`r=OUTER; v=VEE; nameref r=v; unset -n r; echo "[${r-GONE}]"`,
			"[GONE]\n",
		},
		{
			"an inherited value goes with it",
			`export E=ENV; v=VEE; typeset -n E=v; unset -n E; echo "[${E-GONE}]"`,
			"[GONE]\n",
		},
		{
			// What the opened cell is: an ordinary local of the keyword
			// function, so a write stays in the call.
			"the opened cell is an ordinary local",
			`r=OUTER; v=VEE; function f { typeset -n r=v; unset -n r; r=NEW; echo "in=[$r]"; }; ` +
				`f; echo "after=[${r-GONE}]"`,
			"in=[NEW]\nafter=[OUTER]\n",
		},
		{
			"and the target is untouched",
			`v=VEE; typeset -n r=v; unset -n r; echo "[${r-GONE}] v=[$v]"`,
			"[GONE] v=[VEE]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The other shape of the same refusal. This column looks at the array
// **first** — ahead of the bad-target and self-reference refusals, which bash
// puts in front of it — and only a name that really *holds* an array answers:
// a bare `typeset -a r` is an attribute and nothing more here, so it takes a
// reference that bash refuses, while `typeset -A r` is an object this shell's
// own listing prints as `typeset -A m=()` and is refused.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01, `env -i` with a scratch HOME and
// no startup files, every row run against the real shell and this one side by
// side. The refusal ends the script because `typeset` is one of this shell's
// special builtins, which is BadNameToDeclarationFatal and not a second
// disagreement about this rule. See Semantics.NamerefArrayRefusal (#3103).
func TestAReferenceOverAnArrayIsRefusedFirstAndOnTheContents(t *testing.T) {
	dir := t.TempDir()
	const refused = "ksh: typeset: r: reference variable cannot be an array\n"
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			// The refusal, and the script ending on it: nothing after the
			// declaration runs, which is how this column differs from bash
			// on every row below that has an `echo` behind it.
			name: "an indexed array is refused and ends the script",
			src:  `r=(a b c); v=VEE; typeset -n r=v; echo after`,
			want: refused, status: 1,
		},
		{
			name: "the associative kind too", src: `typeset -A r=([k]=KV); v=VEE; typeset -n r=v; echo after`,
			want: refused, status: 1,
		},
		{
			// The **contents** half of the axis, and the row bash answers
			// the other way: the attribute alone is not an array here, so
			// the reference is made and reads through.
			name: "a bare indexed attribute takes the reference",
			src:  `typeset -a r; v=VEE; typeset -n r=v; echo "st=$? [$r]"; typeset -p r`,
			want: "st=0 [VEE]\ntypeset -n r=v\n",
		},
		{
			// And what that leaves behind: the attribute goes with the
			// value, so the cell the reference took over is empty
			// underneath it rather than still an array (#3084).
			name: "and the attribute goes with the value",
			src:  `typeset -a r; v=VEE; typeset -n r=v; unset -n r; echo "[${r-GONE}]"; typeset -p r 2>&1`,
			want: "[GONE]\n",
		},
		{
			// An element landing in it makes it one, which is the line
			// between the two readings.
			name: "one element makes it an array",
			src:  `typeset -a r; r[0]=x; v=VEE; typeset -n r=v; echo after`,
			want: refused, status: 1,
		},
		{
			// The bare *associative* attribute is an object here and is
			// refused, where the indexed one is not — this shell's own
			// listing is what says so.
			name: "the bare associative attribute is an object",
			src:  `typeset -A r; v=VEE; typeset -n r=v; echo after`,
			want: refused, status: 1,
		},
		{
			// The valueless form asks no axis: it is refused on the
			// attribute alone, in both shells, and this is the row that
			// says so — the same `typeset -a r` that takes `=v` above.
			name: "the valueless form refuses the bare attribute",
			src:  `typeset -a r; typeset -n r; echo after`,
			want: refused, status: 1,
		},
		{
			// The **ordering** half, twice: with the array there it speaks
			// for the line, where bash reports the name instead.
			name: "the array is reported ahead of a bad target",
			src:  `r=(a b); typeset -n r=1bad; echo after`,
			want: refused, status: 1,
		},
		{
			name: "and ahead of a self reference", src: `r=(a b); typeset -n r=r; echo after`,
			want: refused, status: 1,
		},
		{
			// With no array to report, the same two lines fall through to
			// the refusals they would have had — which is what says the
			// ordering is the difference rather than the array check
			// swallowing them.
			name: "a bare attribute lets the self reference through",
			src:  `typeset -a r; typeset -n r=r; echo after`,
			want: "ksh: typeset: r: invalid self reference\n", status: 1,
		},
		{
			// `typeset` in a POSIX-style function declares no local here, so
			// the name a reference lands on is the global one and the array
			// is still in it — the scope axis this dialect already carries,
			// where bash's fresh local binding holds no array and takes the
			// line.
			name: "a POSIX function reaches the global array",
			src:  `r=(a b c); v=VEE; f(){ typeset -n r=v; echo in; }; f; echo after`,
			want: refused, status: 1,
		},
		{
			// And a keyword function, which does get a scope: the binding
			// is fresh and holds nothing, so it is taken.
			name: "a keyword function gets a fresh binding",
			src: `r=(a b c); v=VEE; function f { typeset -n r=v; echo "in_st=$? in=[$r]"; }; f; ` +
				`echo "after=[${r-GONE}] n=${#r[@]}"`,
			want: "in_st=0 in=[VEE]\nafter=[a] n=3\n",
		},
		{
			// The second spelling reaches the same rule, and the sentence
			// still names `typeset` — measured, not assumed — which is what
			// says the word is a front and not a builtin of its own.
			name: "nameref is refused in typeset's name",
			src:  `r=(a b); v=VEE; nameref r=v; echo after`,
			want: refused, status: 1,
		},
		{
			// The *target* being an array is the feature, not the question.
			name: "a reference aimed at an array is fine",
			src:  `v=(a b c); typeset -n r=v; echo "st=$? ${r[1]} ${#r[@]}"`,
			want: "st=0 b 3\n",
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
