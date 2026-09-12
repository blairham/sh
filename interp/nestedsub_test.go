// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A subscript applied to what a nested expansion came to — `${${a[@]}[2]}`.
//
// Measured on zsh 5.9.2, the only panel shell with the grammar; the tests
// here name the grammar flag and the axes and never the shell, and
// dialect/zsh carries the end-to-end half.
//
// The subscript's own readings are already the subject of their own tests —
// the base, the range, the search letters, the character reading of a string.
// What is asserted here is the one question this construct adds: whether the
// inner's result is a *list*, where the subscript counts elements, or one
// string, where it counts characters.

// nestedSubscriptGrammar is the grammar this construct needs: the nesting,
// and a subscript that may carry a flag group.
func nestedSubscriptGrammar(d *syntax.Dialect) {
	nesting(d)
	d.ArraySubscriptFlags = true
	d.ParamSplitFlag = true
	d.ParamSetTestFlag = true
}

// runNestedSubscript runs src with that grammar and with every axis the
// *reading* depends on pinned, so that a test asserting the nesting is not
// also asserting one of them by accident. Each is the subject of its own
// tests elsewhere: where an array starts, whether a comma in a subscript is a
// range, whether a subscript on a string names a character, whether a bare
// array name is its elements, and whether an unquoted expansion is split.
func runNestedSubscript(t *testing.T, src string, set ...func(*Semantics)) (string, int) {
	t.Helper()
	return runGrammar(t, src, nestedSubscriptGrammar, func(r *Runner) {
		// The same grammar for input the *run* parses — an arithmetic
		// operand is re-lexed there, and `(( ${${(P)h}[(I)x]} == 0 ))` is
		// the construct's headline use.
		d := syntax.Core()
		nestedSubscriptGrammar(&d)
		r.Dialect = &d
		sem := *r.Semantics
		sem.ArrayBaseIsZero = No
		sem.SubscriptCommaIsARange = Yes
		sem.ScalarSubscriptIsACharacter = Yes
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.SplitParamExpansion = No
		for _, f := range set {
			f(&sem)
		}
		r.Semantics = &sem
	})
}

// The elements of a result that is a list, counted from the base.
func TestASubscriptOnANestedResultCountsItsElements(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an element by its subscript", `a=(x y z); printf "[%s]" "${${a[@]}[2]}"`, "[y]"},
		{"the first", `a=(x y z); printf "[%s]" "${${a[@]}[1]}"`, "[x]"},
		{"counting back from the end", `a=(x y z); printf "[%s]" "${${a[@]}[-1]}"`, "[z]"},
		{"below the base is no element", `a=(x y z); printf "[%s]" "${${a[@]}[0]}"`, "[]"},
		{"past the end is no element", `a=(x y z); printf "[%s]" "${${a[@]}[9]}"`, "[]"},
		{"the bare name is the list too", `a=(x y z); printf "[%s]" ${${a}[2]}`, "[y]"},
		// The subscript is an expression and not a numeral, exactly as it is
		// on a name: this is the same code, and the row is here to say so.
		{"an expression as the subscript", `a=(x y z); i=1; printf "[%s]" "${${a[@]}[i+1]}"`, "[y]"},
		{"an expansion in the subscript", `a=(x y z); j=3; printf "[%s]" "${${a[@]}[$j]}"`, "[z]"},
		// An element holding a space is one element, which is the whole
		// reason the subscript reads elements rather than words.
		{"an element holding a space", `a=("p q" z); printf "[%s]" "${${a[@]}[1]}"`, "[p q]"},
		// The outer operator then applies to what the subscript selected.
		{"an operator over the element", `a=(abc z); printf "[%s]" "${${a[@]}[1]#a}"`, "[bc]"},
		{"a length of the element", `a=(abc z); printf "[%s]" "${#${a[@]}[1]}"`, "[3]"},
		{"a flag group over the element", `a=(abc z); printf "[%s]" "${(U)${a[@]}[1]}"`, "[ABC]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNestedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A result that is one *string* is read by character, which is the same
// question `${s[2]}` asks of a name — and it is asked of the axis rather than
// assumed, because two dialects read a subscripted string two ways.
func TestASubscriptOnANestedStringNamesACharacter(t *testing.T) {
	const src = `v=abc; printf "[%s]" "${${v}[2]}"`
	if out, st := runNestedSubscript(t, src); out != "[b]" || st != 0 {
		t.Errorf("as characters: got %q (status %d), want [b] at 0", out, st)
	}
	// The other answer reads the string as an array of one, where only the
	// base names it — so the same text is no value at all.
	out, st := runNestedSubscript(t, src, func(s *Semantics) {
		s.ScalarSubscriptIsACharacter = No
	})
	if out != "[]" || st != 0 {
		t.Errorf("as an element: got %q (status %d), want [] at 0", out, st)
	}
	// A range over a string is a substring, one value however many
	// characters it holds.
	if out, st := runNestedSubscript(t, `v=abcde; printf "[%s]" "${${v}[2,4]}"`); out != "[bcd]" || st != 0 {
		t.Errorf("a range: got %q (status %d), want [bcd] at 0", out, st)
	}
}

// The discriminating pair, and the reason the shape is a question about the
// inner rather than about how many fields it produced: a list holding one
// element and a string are both one field, and the subscript reads them
// differently.
func TestOneFieldIsNotEnoughToSayWhichReadingApplies(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// A one-element list has no second element, where the string `abc`
		// has a second character.
		{"a list of one stays a list", `a=(abc); printf "[%s]" ${${a[@]}[2]}`, "[]"},
		{"a string stays a string", `v=abc; printf "[%s]" ${${v}[2]}`, "[b]"},
		// A split that found nothing to split leaves a string, and a join
		// leaves one as well — both measured.
		{"a split with one field is a string", `v=abc; printf "[%s]" ${${(f)v}[2]}`, "[b]"},
		// Over an *array*, which is the discriminating half: the split joins
		// its elements first and what it leaves is a string, where the name
		// on its own is a list.
		{"a split over an array leaves a string", `a=(hello); printf "[%s]" ${${(f)a}[2]}`, "[e]"},
		{"an `s` split over one too", `a=(hello); printf "[%s]" ${${(s.,.)a}[2]}`, "[e]"},
		{"and a `${=v}` split", `a=(hello); printf "[%s]" ${${=a}[2]}`, "[e]"},
		{"a join is a string", `a=(pq rs); printf "[%s]" ${${(j.,.)a}[3]}`, "[,]"},
		// A flag that neither splits nor joins keeps the list it was given.
		{"a case flag keeps the list", `a=(abc); printf "[%s]" ${${(U)a}[2]}`, "[]"},
		// A count is a string, whatever it counted.
		// A count of ten or more is where the two readings part: `10` has a
		// second character and a list of one has no second element.
		{"a count is a string", `a=(1 2 3 4 5 6 7 8 9 10); printf "[%s]" ${${#a[@]}[2]}`, "[0]"},
		// One element selected out of an array is that element, read as the
		// string it is.
		{"an element is a string", `a=(abc); printf "[%s]" ${${a[1]}[2]}`, "[b]"},
		// And a conditional's substituted word is its own value: this one is
		// a string, the next is a list.
		{"a substituted word decides for itself", `a=(x y z); printf "[%s]" ${${a:+abc}[2]}`, "[b]"},
		{"a substituted list stays a list", `a=(abc); unset u; printf "[%s]" ${${u:-$a}[2]}`, "[]"},
		// The word's *own* spans decide, so a scalar in it is a string and
		// quoting a list in it makes one.
		{"a substituted scalar is a string", `v=abc; unset u; printf "[%s]" ${${u:-$v}[2]}`, "[b]"},
		{"a substituted quoted list joins", `a=(x y z); unset u; printf "[%s]" ${${u:-"$a"}[2]}`, "[ ]"},
		// `(A)` says outright that what it made is an array, where the value
		// it was given was a string.
		{"the (A) flag makes a list", `v=abc; printf "[%s]" ${${(A)v}[2]}`, "[]"},
		// The other two splits, which reach the same rule by other letters.
		{"an `s` split with one field is a string", `v=abc; printf "[%s]" ${${(s.,.)v}[2]}`, "[b]"},
		{"an `=` split with one field is a string", `v=abc; printf "[%s]" ${${=v}[2]}`, "[b]"},
		// A bare array name of one element, which is the list reading
		// reached without a subscript on the inner.
		{"a bare name of one element is a list", `a=(abc); printf "[%s]" ${${a}[2]}`, "[]"},
		// And the depth does not make a list of a string.
		{"a nested inner is still a string", `v=abc; printf "[%s]" ${${${v}}[2]}`, "[b]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNestedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A search subscript over the result: the letters select by *searching*, and
// the answer is an index or an element exactly as it is on a name.
func TestASearchSubscriptOverANestedResult(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the index of the last match", `a=(x y z); printf "[%s]" "${${a[@]}[(I)y]}"`, "[2]"},
		{"the index of the first", `a=(y y); printf "[%s]" "${${a[@]}[(i)y]}"`, "[1]"},
		{"the element itself", `a=(x y z); printf "[%s]" "${${a[@]}[(r)y]}"`, "[y]"},
		{"the last such element", `a=(ya yb); printf "[%s]" "${${a[@]}[(R)y*]}"`, "[yb]"},
		{"a pattern rather than a string", `a=(x yy z); printf "[%s]" "${${a[@]}[(r)y*]}"`, "[yy]"},
		{"an element none matches", `a=(x y z); printf "[%s]" "${${a[@]}[(r)zz]}"`, "[]"},
		// The two ends a failed search answers with, which is what makes
		// `(i)` an append position and `(I)` a not-found marker.
		{"no match is one before the base", `a=(x y z); printf "[%s]" "${${a[@]}[(I)zz]}"`, "[0]"},
		{"no match is one past the end", `a=(x y z); printf "[%s]" "${${a[@]}[(i)zz]}"`, "[4]"},
		{"an empty list answers both ends", `a=(); printf "[%s][%s]" "${${a[@]}[(I)zz]}" "${${a[@]}[(i)zz]}"`, "[0][1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNestedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The index a failed search answers with is *zero*, and the caller reads it
// as arithmetic rather than as text.
//
// This is the whole point of the construct's headline use — a hook installer
// asks `(( ${${(P)hook}[(I)$fn]} == 0 ))` before adding a function to a hook
// — and it is asserted through the arithmetic on purpose. An implementation
// that answered *empty* for a search that found nothing would satisfy a test
// comparing the expansion's text against `0` in some spellings and would
// still leave `(( … == 0 ))` a bad math expression with an empty operand,
// which is the shape the caller actually breaks in.
func TestNoMatchIsZeroToTheArithmeticThatReadsIt(t *testing.T) {
	const src = `a=(x y z); h=a
if (( ${${(P)h}[(I)nosuch]} == 0 )); then echo absent; else echo present; fi
if (( ${${(P)h}[(I)y]} == 0 )); then echo absent; else echo present; fi`
	out, st := runNestedSubscript(t, src)
	if out != "absent\npresent\n" || st != 0 {
		t.Errorf("got %q (status %d), want absent then present at 0", out, st)
	}
	// And the same reading over the result rather than through a name, so
	// the two routes cannot answer the test differently.
	out, st = runNestedSubscript(t, `a=(x y z)
if (( ${${a[@]}[(I)nosuch]} == 0 )); then echo absent; fi`)
	if out != "absent\n" || st != 0 {
		t.Errorf("over the result: got %q (status %d), want absent at 0", out, st)
	}
}

// `${(P)h}` is a *name*, so the subscript reads the parameter it names rather
// than the fields that parameter would expand to. Three things follow, and
// none of them follows from the fields.
func TestASubscriptThroughAParameterReferenceReadsTheParameter(t *testing.T) {
	// One element of a list of one, which a character reading would have
	// answered `y` for.
	if out, st := runNestedSubscript(t, `a=(xyz); h=a; printf "[%s]" "${${(P)h}[2]}"`); out != "[]" || st != 0 {
		t.Errorf("a list of one: got %q (status %d), want [] at 0", out, st)
	}
	// A scalar the name reaches is read as characters, exactly as `${v[2]}`
	// is: the reference carries the parameter's own shape.
	if out, st := runNestedSubscript(t, `v=abc; h=v; printf "[%s]" "${${(P)h}[2]}"`); out != "[b]" || st != 0 {
		t.Errorf("a scalar: got %q (status %d), want [b] at 0", out, st)
	}
	// An element holding the empty string is *there*, where the fields of
	// the same array have dropped it.
	out, st := runNestedSubscript(t, `a=("" y); h=a; printf "[%s][%s]" "${${(P)h}[1]-none}" "${${(P)h}[9]-none}"`)
	if out != "[][none]" || st != 0 {
		t.Errorf("an empty element: got %q (status %d), want [][none] at 0", out, st)
	}
	// And the name may be one the reference computed, at any depth.
	if out, st := runNestedSubscript(t,
		`a=(x y z); h=a; g=h; printf "[%s]" "${${(P)${(P)g}}[2]}"`); out != "[y]" || st != 0 {
		t.Errorf("through two references: got %q (status %d), want [y] at 0", out, st)
	}
}

// The name a `(P)` resolves to is the inner expansion with `P` struck out of
// its group, and the letters left transform the *name* rather than what the
// name holds.
//
// The discriminating pair is two parameters whose names differ only in case:
// with `ARR=(x y)`, `arr=(hello)` and `h=arr`, `${(UP)h}` on its own is
// `HELLO` — the value, uppercased — where the same group *subscripted* is
// `${ARR[1]}`, which is `x`. An implementation that resolved the name first
// and applied the letters afterwards answers `hello` and `h` respectively,
// both of them plausible.
func TestTheOtherLettersBesideAReferenceTransformTheName(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the uppercased name is the parameter", `ARR=(x y); arr=(hello); h=arr; printf "[%s]" "${${(UP)h}[1]}"`, "[x]"},
		{"and its other elements", `ARR=(x y); arr=(hello); h=arr; printf "[%s]" "${${(UP)h}[2]}"`, "[y]"},
		{"a name nothing is stored under is empty", `arr=(hello); h=arr; printf "[%s]" "${${(UP)h}[1]}"`, "[]"},
		{"the group's own base is resolved first", `H=(m n); h=arr; g=h; printf "[%s]" "${${(UP)${g}}[2]}"`, "[n]"},
		{"an operator runs on the name too", `a=(x y z); h=a; printf "[%s]" "${${(P)h:-d}[2]}"`, "[y]"},
		{"and its word where the test fires", `a=(x y z); unset h; printf "[%s]" "${${(P)h:-a}[2]}"`, "[y]"},
		{"a search through a transformed name", `ARR=(x y z); arr=(q); h=arr; printf "[%s]" "${${(UP)h}[(I)y]}"`, "[2]"},
		// A count and a set test are names too, which is the sharp end of
		// "the whole pipeline": `${#hh}` is 2, so the subscript reads the
		// *second positional parameter* — and a guard excluding them looked
		// obviously right and was wrong.
		{"a count is a name", `set -- abc def; hh=zz; printf "[%s]" "${${(P)#hh}[1]}"`, "[d]"},
		{"and a set test is one", `set -- abc; h=zz; printf "[%s]" "${${(P)+h}[1]}"`, "[a]"},
		{"a name nothing answers to is empty", `set -- abc; h=zz; printf "[%s]" "${${(P)#h}[1]}"`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNestedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The reference reaches an association's key, which is the sharpest half:
// an association's *fields* are its values, and a key is no position in them.
func TestAParameterReferenceSubscriptReadsAKey(t *testing.T) {
	out, st := runGrammar(t, `h=m; printf "[%s][%s]" "${${(P)h}[k]}" "${${(P)h}[nosuch]-none}"`,
		nestedSubscriptGrammar, func(r *Runner) {
			r.AssocArrays = map[string]AssocArray{"m": {"k": "v"}}
		})
	if out != "[v][none]" || st != 0 {
		t.Errorf("got %q (status %d), want [v][none] at 0", out, st)
	}
	// Written as a *value* instead, the same table is its values — a list,
	// which is what the subscript then counts through, so a one-key table
	// has no second element where its one value has a second character.
	out, st = runGrammar(t, `printf "[%s]" ${${m}[2]}`, nestedSubscriptGrammar, func(r *Runner) {
		r.AssocArrays = map[string]AssocArray{"m": {"k": "abc"}}
	})
	if out != "[]" || st != 0 {
		t.Errorf("as a value: got %q (status %d), want [] at 0", out, st)
	}
}

// A subscript that named nothing leaves the expansion *unset*, which is what
// the conditionals then test — where an element that is there and empty is
// set, and only the reference route can show one, the fields of an unquoted
// expansion having dropped it.
func TestASubscriptThatNamedNothingIsUnset(t *testing.T) {
	out, st := runNestedSubscript(t,
		`a=(x y z); printf "[%s][%s]" "${${a[@]}[2]-none}" "${${a[@]}[9]-none}"`)
	if out != "[y][none]" || st != 0 {
		t.Errorf("got %q (status %d), want [y][none] at 0", out, st)
	}
}

// Quoting reaches the inner, and it is what decides whether an inner that
// came to a list still has fields for the subscript to count.
//
// One predicate covers it — `@` itself, an `[@]` subscript and the `(@)` flag
// keep their fields inside quotes, and everything else joins — which is the
// same rule the rest of the expansion machinery follows for `"$@"` against
// `"$*"`. The join is on IFS rather than on a hard space, which is the half a
// test can tell apart from a plausible one.
func TestQuotingReachesTheInnerExpansion(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare name joins in quotes", `a=(p q r); printf "[%s]" "${${a}[2]}"`, "[ ]"},
		{"and joins on IFS", `IFS=-; a=(p q r); printf "[%s]" "${${a}[2]}"`, "[-]"},
		{"an `[@]` subscript keeps its fields", `a=(p q r); printf "[%s]" "${${a[@]}[2]}"`, "[q]"},
		{"`@` keeps them too", `set -- p q r; printf "[%s]" "${${@}[2]}"`, "[q]"},
		{"`*` does not", `set -- p q r; printf "[%s]" "${${*}[2]}"`, "[ ]"},
		{"the `(@)` flag keeps them", `a=(p q r); printf "[%s]" "${${(@)a}[2]}"`, "[q]"},
		{"another flag does not", `a=(p q r); printf "[%s]" "${${(o)a}[2]}"`, "[ ]"},
		// A split still runs after the join, so its fields survive quoting.
		// A count is a string in quotes too, which is a rule of its own:
		// `flagKeepsFields` would say an `[@]` subscript keeps its fields,
		// and a count is not its parameter's shape whatever the subscript
		// under it says.
		{"a count in quotes is still a string", `a=(1 2 3 4 5 6 7 8 9 10); printf "[%s]" "${${#a[@]}[2]}"`, "[0]"},
		{"a split keeps its fields", `v="aa
bb"; printf "[%s]" "${${(f)v}[2]}"`, "[bb]"},
		// An inner written with quotes of its own is the same rule, reached
		// by the quotes it carries rather than by the ones around it.
		{"an inner quoted on its own joins", `a=(p q r); printf "[%s]" ${"${a}"[2]}`, "[ ]"},
		{"and an `[@]` inside them keeps fields", `a=(p q r); printf "[%s]" ${"${a[@]}"[2]}`, "[q]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNestedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A command substitution and an arithmetic one stand in the same position,
// and both are a **list** there even when they come to one word — which is
// the half a field count cannot guess, and which needed the name position to
// field-split before it could be read at all (#1394, behind #976).
//
// Measured on zsh 5.9.2, 2026-09-12. Rows two and six are the discriminating
// ones: a source read as a *string* answers `b` and `2` there, which is a
// plausible value at status 0 — the failure the refusal that used to stand
// here existed to prevent.
func TestASubscriptOnANestedSubstitution(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a list of three, the second", `printf "[%s]" ${$(echo a b c)[2]}`, "[b]"},
		{"a list of one has no second", `printf "[%s]" ${$(echo abc)[2]}`, "[]"},
		{"and its first is the whole word", `printf "[%s]" ${$(echo abc)[1]}`, "[abc]"},
		{"an empty list, searched", `printf "[%s]" ${$(true)[(I)x]}`, "[0]"},
		{"arithmetic is a list of one too", `printf "[%s]" ${$((6*7))[1]}`, "[42]"},
		{"so it has no second either", `printf "[%s]" ${$((6*7))[2]}`, "[]"},
		{"a range takes elements", `printf "[%s]" ${$(echo a b c)[2,3]}`, "[b][c]"},
		{"counting from the end", `printf "[%s]" ${$(echo a b c)[-1]}`, "[c]"},
		{"a search takes one", `printf "[%s]" ${$(echo a b c)[(r)b]}`, "[b]"},
		{"and the whole of it", `printf "[%s]" ${$(echo a b c)[@]}`, "[a][b][c]"},
		{"a length measures the element", `printf "[%s]" ${#$(echo a b c)[2]}`, "[1]"},
		{"and counts them under [@]", `printf "[%s]" ${#$(echo a b c)[@]}`, "[3]"},
		// Quoted it is a string, and that is the parameter inner's rule
		// reached by the same route rather than one of its own: the quotes
		// joined the fields before the subscript saw them.
		{"quoted, the subscript counts characters", `printf "[%s]" "${$(echo a b c)[2]}"`, "[ ]"},
		{"an inner quoted on its own, the same", `printf "[%s]" ${"$(echo abc)"[2]}`, "[b]"},
		{"and an arithmetic one", `printf "[%s]" "${$((6*7))[1]}"`, "[4]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNestedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A substitution in that position that came to nothing is an **empty list**
// and not one empty field, which only a count can see: measured on zsh 5.9.2,
// `${#$(true)[@]}` and `${#$(true)}` are both 0, where one empty field would
// have answered 1 to both.
func TestAnEmptyNestedSubstitutionIsAnEmptyList(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"no element to count", `printf "[%s]" ${#$(true)[@]}`, "[0]"},
		{"and none to measure", `printf "[%s]" ${#$(true)}`, "[0]"},
		{"nor one to index", `printf "[%s]" ${$(true)[1]}`, "[]"},
		{"while an empty scalar inner is still one field", `s=""; printf "[%s]" ${#${s}[@]}`, "[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNestedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A subscript that names several of the inner's elements answers with all of
// them, and quoting decides whether they stay separate exactly as it does on
// a name.
//
// The `[@]` of the *outer* is what keeps the fields here — measured on zsh
// 5.9.2, `"${${a[@]}[@]}"` is one field per element where `"${${a[@]}}"` with
// no subscript is one field holding the join, and a range joins like every
// spelling that is not `@`. Both were refused by name until #1509; the
// assertions are on the elements because a list of one and a list of three
// differ in a count as well, and a count alone passes against either.
func TestASubscriptOnANestedListNamesEveryElementItSelects(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"quoted, one field each", `a=(x y z); printf "[%s]" "${${a[@]}[@]}"`, "[x][y][z]"},
		{"and the space inside one survives", `a=("p q" z); printf "[%s]" "${${a[@]}[@]}"`, "[p q][z]"},
		{"unquoted, one field each", `a=(x y z); printf "[%s]" ${${a[@]}[@]}`, "[x][y][z]"},
		{"the joining spelling joins", `a=(x y z); printf "[%s]" "${${a[@]}[*]}"`, "[x y z]"},
		{"a range keeps what it selected", `a=(x y z); printf "[%s]" ${${a[@]}[1,2]}`, "[x][y]"},
		{"and joins it in quotes", `a=(x y z); printf "[%s]" "${${a[@]}[1,2]}"`, "[x y]"},
		{"a length counts what was selected", `a=(x y z); printf "n=%s" ${#${a[@]}[@]}`, "n=3"},
		// One element is still the list's reading and not the string's: a
		// character count would answer 5 here.
		{"one element counts as one", `a=(hello); printf "n=%s" ${#${a[@]}[@]}`, "n=1"},
		// A range that selected one element is still the *list's* reading,
		// which is where the count says which reading ran: measured,
		// `${#${a[@]}[1,1]}` on `(hello world)` is 1 and not 5.
		{"a one-element range counts as one", `a=(hello world); printf "n=%s" ${#${a[@]}[1,1]}`, "n=1"},
		// And the same range over a *string* is the string's reading, which
		// is the other side of that line: three characters, not one field.
		{"a range over a string counts characters", `s=hello; printf "n=%s" ${#${s}[2,4]}`, "n=3"},
		// A subscript naming *one* element is that element, unchanged — the
		// reading this file's other tests are about.
		{"one element is still one element", `a=(x y z); printf "[%s]" "${${a[@]}[2]}"`, "[y]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNestedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// An expansion the grammar could not read is not a reference to anything.
//
// `@Q` is not in this grammar, so the node is marked unreadable and the run
// reports it — where taking it as a reference would resolve the name its base
// happened to hold and answer with an element of *that*, at status 0. It is
// the same guard `expandAtList` keeps for the same reason, one construct
// over: a `Bad` node is not a shape.
func TestAnUnreadableInnerIsNotAReference(t *testing.T) {
	out, st := runNestedSubscript(t, `a=(x y); h=a; printf "[%s]" "${${(P)h@Q}[1]}"`)
	if !strings.Contains(out, "bad substitution") {
		t.Errorf("got %q, want the unreadable inner reported", out)
	}
	if st == 0 {
		t.Errorf("status 0, want the unreadable inner refused")
	}
}

// A refusal from inside the reference route names what the *script* wrote
// rather than the name the reference resolved to.
//
// `${${(P)h}[(w)y]}` with `h=a` reads the subscript against `a`, so a refusal
// naming its subject would say `a[(w)y]` — a construct the file does not
// contain, and one a reader cannot search for.
// A comma written in a subscript separates a *pair* even inside what looks
// like a search's operand, and the search gets only the half in front of it.
//
// This asserted the other reading — that `(r)1,2` is one operand naming the
// element whose value is `1,2`, a width of 3 — with a comment recording that
// the shell answered neither 3 nor a count of matches and calling that the
// operand's business. It is the pair's: measured on zsh 5.9.2,
// `a=(p q,r s); ${a[(r)q,r]}` is empty rather than the element holding the
// comma, so the search is for `q` alone and `r` is the other end. Here the
// search for `1` misses, which is one past the last element, and `2` is an
// end before it — an empty span, and a length of 0 (#1533).
//
// The nesting is what makes it worth a row of its own: a subscript on an
// expansion's result reaches the same split as one on a name.
func TestACommaInASearchOperandSeparatesThePair(t *testing.T) {
	out, st := runNestedSubscript(t, `a=("1,2" x y); printf "[%s]" "${#${a[@]}[(r)1,2]}"`)
	if want := "[0]"; out != want || st != 0 {
		t.Errorf("gave %q at %d, want %q at 0 — an empty span, not the element holding the comma", out, st, want)
	}
}

func TestARefusalThroughAReferenceNamesTheWrittenText(t *testing.T) {
	out, st := runNestedSubscript(t, `a=(x y); h=a; printf "[%s]" "${${(P)h}[(w)y]}"`)
	if !strings.Contains(out, "${${(P)h}[(w)y]}: the (w) subscript flag is not implemented") {
		t.Errorf("got %q, want the refusal to name the written text", out)
	}
	if st == 0 {
		t.Errorf("status 0, want the unimplemented flag refused")
	}
}

// Without the subscript grammar the same characters are not this shape at
// all: the brackets are part of the operand and the expansion is refused
// rather than read one dialect's way.
func TestANestedSubscriptNeedsTheSubscriptGrammar(t *testing.T) {
	out, st := runGrammar(t, `a=(x y z); printf "[%s]" "${${a[@]}[2]}"`, func(d *syntax.Dialect) {
		nestedSubscriptGrammar(d)
		d.ArraySubscript = false
	}, nil)
	if st == 0 {
		t.Errorf("status 0, want the expansion refused without the subscript grammar")
	}
	if !strings.Contains(out, "bad substitution") {
		t.Errorf("got %q, want a bad substitution", out)
	}
}

// TestASubscriptOnAConditionalNestingStillRunsTheInnerOnce is #1404 reached
// through the subscript rather than through the plain shape: the conditional
// asks whether its test fires before the subscript names anything, and that
// question used to expand the inner a second time.
//
// The count is the assertion. `b` came back whether the command ran once or
// twice, which is why the issue could stand with the value correct.
func TestASubscriptOnAConditionalNestingStillRunsTheInnerOnce(t *testing.T) {
	out, st := runNestedSubscript(t,
		`printf "[%s]" "${${$(printf x >>marks; echo abc):-d}[2]}"; printf "n=%s" "$(cat marks)"`)
	if want := "[b]n=x"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
