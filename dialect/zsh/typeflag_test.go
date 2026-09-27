// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `${(t)name}` is what a name *is*, in this shell's own words.
//
// The mechanics belong to interp/typeflag_test.go, which supplies a
// vocabulary of its own; these are the *words*, which are this dialect's and
// are the one thing that file cannot assert. They are deliberately the same
// function `$parameters` renders with, so the two spellings of one question
// cannot answer it differently — the last row is what holds that.
//
// Measured against zsh 5.9.2, 2026-09-10; the corpus row is
// `param/expansion-flags-parameter-type`.
func TestTheTypeFlagWordsWhatANameIs(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", `v=abc; print -r -- ${(t)v}`, "scalar\n"},
		{"an array", `w=(a b); print -r -- ${(t)w}`, "array\n"},
		{"an association", `typeset -A m=(k1 v1); print -r -- ${(t)m}`, "association\n"},
		{"an integer", `typeset -i n=1; print -r -- ${(t)n}`, "integer\n"},
		// `-E` is the other float spelling and is refused by name here, so
		// only `-F` is asserted; the day it lands, `${(t)b}` is `float` too.
		{"a float", `typeset -F a=1; print -r -- ${(t)a}`, "float\n"},
		{"exported", `typeset -x e=1; print -r -- ${(t)e}`, "scalar-export\n"},
		{"an exported array", `typeset -xa a=(1); print -r -- ${(t)a}`, "array-export\n"},
		{"readonly before export", `typeset -xr v=1; print -r -- ${(t)v}`, "scalar-readonly-export\n"},
		{"unique after export", `typeset -xaU a=(1); print -r -- ${(t)a}`, "array-export-unique\n"},
		{"local", `f() { local l=1; print -r -- ${(t)l}; }; f`, "scalar-local\n"},
		{"local before readonly", `f() { typeset -ir l=1; print -r -- ${(t)l}; }; f`, "integer-local-readonly\n"},
		{"a tied pair", `typeset -T TV tv; print -r -- ${(t)TV} ${(t)tv}`, "scalar-tied array-tied\n"},
		// Where `tied` stands among the rest, which is **after** the freeze
		// and after the case and width words and **before** everything
		// below them (#4856). Measured 2026-09-27 on zsh 5.9.2 over a script
		// tie, so that `special` is out of the way and the row is about the
		// order alone.
		{"readonly before tied", `typeset -T TA ta; typeset -r ta; print -r -- ${(t)ta}`, "array-readonly-tied\n"},
		{"upper before tied", `typeset -T TB tb; typeset -u tb; print -r -- ${(t)tb}`, "array-upper-tied\n"},
		{"lower before tied", `typeset -T TC tc; typeset -l tc; print -r -- ${(t)tc}`, "array-lower-tied\n"},
		{"a width before tied", `typeset -T TD td; typeset -Z5 td; print -r -- ${(t)td}`, "array-right_zeros-tied\n"},
		{
			"a width and the freeze, both in front of it",
			`typeset -T TE te; typeset -rL5 te; print -r -- ${(t)te}`,
			"array-left-readonly-tied\n",
		},
		// And the four that were already right, which is what says one word
		// moved rather than the list being rewritten: with nothing that
		// outranks `tied` present the two shells agreed all along.
		{"tied before export", `typeset -T TF tf; typeset -x tf; print -r -- ${(t)tf}`, "array-tied-export\n"},
		{"tied before unique", `typeset -T TG tg; typeset -U tg; print -r -- ${(t)tg}`, "array-tied-unique\n"},
		{"tied before hide", `typeset -T TH th; typeset -h th; print -r -- ${(t)th}`, "array-tied-hide\n"},
		{"tied before hideval", `typeset -T TI ti; typeset -H ti; print -r -- ${(t)ti}`, "array-tied-hideval\n"},
		// `local` keeps its place in front of it, which is the row that
		// stops the move going one word too far.
		{
			"local before readonly before tied",
			`f() { typeset -T TJ tj; typeset -r tj; print -r -- ${(t)tj}; }; f`,
			"array-local-readonly-tied\n",
		},
		// And the shell's own tied names, where `special` is on the end.
		{"a frozen shell array", `typeset -ar path; print -r -- ${(t)path}`, "array-readonly-tied-special\n"},
		{"and one with a word on either side of it", `typeset -rU path; print -r -- ${(t)path}`, "array-readonly-tied-unique-special\n"},
		// The two hiding letters are two attributes with two words, which
		// is the whole of #2042: a parameter given only `-H` is `hideval`
		// and not `hide`, so the four rows are the two letters alone, the
		// pair, and where the pair sits among the other words.
		{"the value-hiding letter", `typeset -H hv=1; print -r -- ${(t)hv}`, "scalar-hideval\n"},
		{"the scope-hiding letter", `typeset -h hs=1; print -r -- ${(t)hs}`, "scalar-hide\n"},
		{"hide before hideval", `typeset -hH b=1; print -r -- ${(t)b}`, "scalar-hide-hideval\n"},
		{
			"and both after unique",
			`typeset -rHhU -a c=(1 2); print -r -- ${(t)c}`,
			"array-readonly-unique-hide-hideval\n",
		},
		// The three width letters, which said nothing here until #4504 — a
		// fact the runner already held and nothing carried across the seam.
		// `-Z` and `-R` are two spellings of one *side*, which is why the
		// word names the fill rather than the letter.
		{"the left-justifying width letter", `typeset -L v=ab; print -r -- ${(t)v}`, "scalar-left\n"},
		{"the right-justifying one", `typeset -R w=ab; print -r -- ${(t)w}`, "scalar-right_blanks\n"},
		{"and the zero-filling one, which is the same side", `typeset -Z z=ab; print -r -- ${(t)z}`, "scalar-right_zeros\n"},
		{"a width with no value to learn from still carries the letter", `typeset -L v; print -r -- ${(t)v}`, "scalar-left\n"},
		{"local before a width", `f() { typeset -L l=ab; print -r -- ${(t)l}; }; f`, "scalar-local-left\n"},
		{"a width before unique", `f() { typeset -UL l=ab; print -r -- ${(t)l}; }; f`, "scalar-local-left-unique\n"},
		{"a width before upper", `typeset -uL v=ab; print -r -- ${(t)v}`, "scalar-left-upper\n"},
		{"a width before readonly", `typeset -rL v=ab; print -r -- ${(t)v}`, "scalar-left-readonly\n"},
		{"a width before export", `typeset -xL v=ab; print -r -- ${(t)v}`, "scalar-left-export\n"},
		{"a width before hideval", `typeset -HL v=ab; print -r -- ${(t)v}`, "scalar-left-hideval\n"},
		{"and the numeric attribute keeps its own word", `typeset -iL n=5; print -r -- ${(t)n}`, "integer-left\n"},
		{"the container wins over the numeric attribute", `typeset -ia ia; ia=(1 2); print -r -- ${(t)ia}`, "array\n"},
		{"an unset name is empty, and unset", `unset u; print -r -- "[${(t)u}][${(t)u-D}]"`, "[][D]\n"},
		{
			"and it is the same word the table gives",
			`typeset -xr v=1; w=(a b); print -r -- "[${(t)v}][${parameters[v]}][${(t)w}][${parameters[w]}]"`,
			"[scalar-readonly-export][scalar-readonly-export][array][array]\n",
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

// A **kind letter** written over a name that already has another kind takes
// the old one away here, where the rest of the panel lets the two stand
// together (#4881). See interp.Semantics.KindLetterReplacesTheKind for the
// measured grid and for bash's and ksh93's answers.
func TestAKindLetterReplacesTheKind(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an array under the integer letter", `typeset -a q; typeset -i q; typeset -p q`, "typeset -i q=0\n"},
		{"an array under a float letter", `typeset -a q; typeset -F q; typeset -p q`, "typeset -F q=0.0000000000\n"},
		{"a table under a float letter", `typeset -A q; typeset -F q; typeset -p q`, "typeset -F q=0.0000000000\n"},
		{"an integer under the array letter", `typeset -i q; typeset -a q; typeset -p q`, "typeset -a q=(  )\n"},
		{"an integer under the table letter", `typeset -i q; typeset -A q; typeset -p q`, "typeset -A q=( )\n"},
		{"a float under the array letter", `typeset -F q; typeset -a q; typeset -p q`, "typeset -a q=(  )\n"},
		// The value the container held goes with it, which is the row that
		// says the old kind is dropped rather than converted: `q=1` would be
		// the first element arriving through the scalar view.
		{"and the elements go with it", `typeset -a q=(1 2); typeset -i q; typeset -p q`, "typeset -i q=0\n"},
		{"a table's values too", `typeset -A m=(k v); typeset -i m; typeset -p m`, "typeset -i m=0\n"},
		// Two kind letters on **one** line are a different question already
		// answered elsewhere — the numeric letter takes the container's here
		// and both stand in bash — so what this rule replaces is a kind an
		// *earlier* declaration gave, never one the same line wrote. See
		// Runner.typeLetterTakesTheCompoundLetter.
		{"two letters on one line are not this rule", `typeset -ia q; typeset -p q`, "typeset -i q=0\n"},
		{"whichever order they are written in", `typeset -ai q; typeset -p q`, "typeset -i q=0\n"},
		// A letter that is not a kind leaves the kind alone, which is the
		// control that keeps this from reading as "any second declaration
		// resets the name".
		{"an export letter leaves the array", `typeset -a q; typeset -x q; typeset -p q`, "typeset -ax q=(  )\n"},
		{"a freeze leaves the integer", `typeset -i q=5; typeset -r q; typeset -p q`, "typeset -ir q=5\n"},
		// And one container letter over the other is a conversion with a
		// measured answer of its own, not a deletion by this rule.
		{"a table under the array letter", `typeset -A m=(k v); typeset -a m; typeset -p m`, "typeset -a m=(  )\n"},
		{"an array under the table letter", `typeset -a a=(1 2); typeset -A a; typeset -p a`, "typeset -A a=( )\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// And a **plus** letter takes off the container it names and leaves the other
// kind standing, which is not an axis: zsh and bash 5.3 agree on all four
// rows, bash 3.2 has no `-A` and ksh93 refuses every plus form of either
// (#4881). See declareFlags.containerLetterRemoved.
func TestAPlusContainerLetterTakesOffOnlyItsOwn(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the array letter over an array", `typeset -a q; typeset +a q; typeset -p q`, "typeset q=''\n"},
		{"the table letter over a table", `typeset -A q; typeset +A q; typeset -p q`, "typeset q=''\n"},
		// The pair that says the letter is read: before this either plus
		// form emptied the name, so a script removing the attribute it had
		// not got lost the one it had.
		{"the table letter over an array", `typeset -a q; typeset +A q; typeset -p q`, "typeset -a q=(  )\n"},
		{"the array letter over a table", `typeset -A q; typeset +a q; typeset -p q`, "typeset -A q=( )\n"},
		{"and the values stay with it", `typeset -A m=(k v); typeset +a m; print -r -- ${m[k]}`, "v\n"},
		{"both letters together take both", `typeset -a a=(1 2); typeset +aA a; typeset -p a`, "typeset a=''\n"},
		// The controls: a name that is neither is silent, in both shells.
		{"a scalar is left alone", `q=1; typeset +a q; typeset -p q`, "typeset q=1\n"},
		{"and a name that is not there", `typeset +A nosuch; print st=$?`, "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
