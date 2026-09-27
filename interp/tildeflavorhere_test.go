// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `~(…)` group naming a flavor is read where it stands, and the glob in
// front of it is translated rather than left as the flavor's text.
//
// The rows are findTildeFlavorGroup's and globToRE2's own tables, put to the
// condition surface. Nothing here names a shell: the construct is the
// `TildeGroup` grammar flag and the letters are that construct's.
func TestATildeFlavorGroupInTheMiddleOfAPattern(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The controls, and they are the reason the rest is readable. The
		// first says a flavor at the head is read at all; the second that
		// what it reads is a *substring* of the subject, which is the
		// property every row below inherits.
		{"a flavor at the head", `[[ zab == ~(E)z.b ]] && echo YES || echo NO`, "YES"},
		{"and it matches a substring", `[[ zab == ~(E)a ]] && echo YES || echo NO`, "YES"},

		{"one in the middle is read", `[[ zA == z~(E)A ]] && echo YES || echo NO`, "YES"},
		{"with text behind the match", `[[ zAB == z~(E)A ]] && echo YES || echo NO`, "YES"},

		// **The two rows that fix what the group does to the text in front of
		// it**, and they rule out every reading but the translation. A split
		// whose expression is anchored where the glob stopped answers the
		// second of these no; a split whose expression is searched in what is
		// left answers the first yes. What fits both is that `z` and `A`
		// become one expression `zA`, which is in `zzA` and is not in `zXA`.
		{"the pattern is one expression", `[[ zXA == z~(E)A ]] && echo YES || echo NO`, "NO"},
		{"searched over the whole subject", `[[ zzA == z~(E)A ]] && echo YES || echo NO`, "YES"},

		// And the glob keeps its glob meaning through the translation, which
		// is the other half: a `.` and a `+` in front of the group are the
		// characters they are and not the expression's operators. Each row is
		// a pair, so neither answer can be read as the pattern failing for
		// some other reason.
		{"a period in front is literal", `[[ zXA == z.~(E)A ]] && echo YES || echo NO`, "NO"},
		{"and matches itself", `[[ 'z.A' == z.~(E)A ]] && echo YES || echo NO`, "YES"},
		{"a plus in front is literal", `[[ zaaa == za+~(E)a ]] && echo YES || echo NO`, "NO"},
		{"and matches itself too", `[[ 'za+a' == za+~(E)a ]] && echo YES || echo NO`, "YES"},

		// The two glob operators that do mean something, and a bracket.
		{"a star in front spans", `[[ zqA == z*~(E)A ]] && echo YES || echo NO`, "YES"},
		{"a question mark in front", `[[ zqA == z?~(E)A ]] && echo YES || echo NO`, "YES"},
		{"a bracket in front", `[[ zqA == z[pq]~(E)A ]] && echo YES || echo NO`, "YES"},
		{
			"a negated bracket in front", `[[ zqA == z[!x]~(E)A ]] && echo YES || echo NO`,
			"YES",
		},

		// Every flavor takes a prefix, and the three below are the three
		// shapes tildeRegexAfter joins: an expression taken as it stands, one
		// translated from a basic expression, and one quoted whole.
		{"a basic expression behind it", `[[ 'za+b' == z~(G)a+b ]] && echo YES || echo NO`, "YES"},
		{"a literal flavor behind it", `[[ 'zqa.b' == z?~(F)a.b ]] && echo YES || echo NO`, "YES"},
		{"and a literal is literal", `[[ zqaXb == z?~(F)a.b ]] && echo YES || echo NO`, "NO"},

		// The letters compose across two groups: a fold at the head reaches
		// the expression a flavor group further along compiles.
		{"a fold at the head reaches it", `[[ zA == ~(i)z~(E)a ]] && echo YES || echo NO`, "YES"},

		// Once a flavor is read, what follows is that flavor's text — so a
		// second group behind it is four characters of expression and not a
		// group this shell reads.
		{
			"a second group behind it is text",
			`[[ zA == z~(E)~(i)a ]] && echo YES || echo NO`, "NO",
		},

		// `K` leaves the language where it is, so the group is consumed by
		// the walk and what follows is an ordinary glob.
		{"a group that keeps the glob", `[[ zab == z~(K)a* ]] && echo YES || echo NO`, "YES"},
		{"with a glob in front of it", `[[ zXab == z?~(K)a* ]] && echo YES || echo NO`, "YES"},

		// **A prefix globToRE2 cannot carry is not claimed**, and this row is
		// the control that says so rather than the comment. Inside a pattern
		// group the text in front of the flavor is `@(z`, which is not a glob
		// on its own, so the pattern falls through to the walk and answers
		// what it answered before — ksh93u+ says yes and this says no, which
		// is #4892. Dropping the prefix instead would answer *this* row right
		// and `@(zq~(E)a)` wrong, which is the plausible-wrong-answer shape,
		// so the row is here to fail if anybody reaches for it.
		{
			"a prefix that cannot be translated is not claimed",
			`[[ za == @(z~(E)a) ]] && echo YES || echo NO`, "NO",
		},
		{
			"nor one holding a second group",
			`[[ zqa == z~(i)q~(E)a ]] && echo YES || echo NO`, "NO",
		},
		{
			"with the same pattern at the head as the control",
			`[[ zqa == ~(i)zq~(E)a ]] && echo YES || echo NO`, "YES",
		},

		// A `case` arm is the same pattern language.
		{"a case arm reads it", `case zab in z~(E)a.) echo YES;; *) echo NO;; esac`, "YES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// And the two surfaces that are not a condition: pathname expansion, which
// needs the gate at the top of the walk to read the field as a pattern at
// all, and a trim, which is where the flavor's substring search is visible as
// a span rather than as a yes.
func TestATildeFlavorGroupOnTheOtherSurfaces(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"za", "zb", "zab"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		// The glob controls: a field with no group is a name, and one whose
		// group is at the head is already a pattern.
		{"a plain name", `printf "[%s]" zq`, `[zq]`},
		{"a group at the head", `printf "[%s]" ~(E)z.`, `[za][zab][zb]`},

		{"one in the middle", `printf "[%s]" z~(E).`, `[za][zab][zb]`},
		{"and it narrows", `printf "[%s]" z~(E)b`, `[zb]`},
		{
			"a field nothing matches stands as written",
			`printf "[%s]" z~(E)^a$`, `[z~(E)^a$]`,
		},

		// A trim, where the same substring search shows as how much it took.
		{"a trim with the group at the head", `v=zab; printf "[%s]" "${v#~(E)z.}"`, `[b]`},
		{"and with it in the middle", `v=zab; printf "[%s]" "${v#z~(E).}"`, `[b]`},
		{
			"a substitution reaches it too",
			`v=zabzab; printf "[%s]" "${v//z~(E)./-}"`, `[-b-b]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tildeMid(t, tc.src, func(r *Runner) { r.Dir = dir })
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
