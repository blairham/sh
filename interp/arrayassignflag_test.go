// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// arrayAssigning is the grammar `${(A)name=value}` needs, named by the
// constructs rather than by a shell: a flag group, the `=` split flag beside
// it, the always-assigning operator, and arrays to read the answer back with.
func arrayAssigning(d *syntax.Dialect) {
	d.ParamExpansionFlags = true
	d.ParamSplitFlag = true
	d.ParamAssignAlways = true
	d.ArrayLiteral = true
	d.ArraySubscript = true
}

// The `(A)` flag makes an assignment written inside an expansion an **array**
// assignment, over the words the value comes to.
//
// Every row is a measurement on zsh 5.9.2 under `-f`, 2026-09-26 — the only
// shell in the panel whose grammar has the flag at all.
//
// The elements are printed one per field rather than counted, because a count
// cannot tell a two-element array from a scalar holding the same characters:
// `${#u}` is the length of a scalar and the element count of an array, so
// `x y` and `( x y )` both read 3 and 2 respectively with nothing in common
// to compare.
func TestTheArrayFlagAssignsAnArray(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The value is one word unless something splits it, so the plain
		// form leaves a **one-element** array and not two.
		{"a value with a blank in it is one element", `unset u; : ${(A)u=x y}; printf "<%s>" "${u[@]}"`, "<x y>"},
		{"and the `=` beside the flag splits it", `unset u; : ${(A)=u=x y}; printf "<%s>" "${u[@]}"`, "<x><y>"},
		{"a run of blanks is one separator", `unset u; : ${(A)=u=a  b}; printf "<%s>" "${u[@]}"`, "<a><b>"},
		{"and IFS decides what a separator is", `unset u; IFS=:; : ${(A)=u=a:b:c}; printf "<%s>" "${u[@]}"`, "<a><b><c>"},
		// Quoting inside the value protects it from that split, which is the
		// ordinary rule and is what says this is field splitting rather than
		// a split of the finished text.
		{"quoted text does not split", `unset u; : ${(A)=u="x y"}; printf "<%s>" "${u[@]}"`, "<x y>"},
		{"nor a quoted run inside a word", `unset u; : ${(A)=u=x"  "y}; printf "<%s>" "${u[@]}"`, "<x  y>"},
		{"nor a quoted parameter", `unset u; s="p q"; : ${(A)=u="$s"}; printf "<%s>" "${u[@]}"`, "<p q>"},
		{"where the same parameter unquoted does", `unset u; s="p q"; : ${(A)=u=$s}; printf "<%s>" "${u[@]}"`, "<p><q>"},
		// A value that is *already* several words needs no `=` at all: the
		// fields are the operand's own.
		{"a list in the value is its own elements", `unset u; a=(1 2); : ${(A)u=${a[@]}}; printf "<%s>" "${u[@]}"`, "<1><2>"},
		{"and an empty list leaves no elements", `unset u; a=(); : ${(A)u=${a[@]}}; printf "[%s]" "${#u}"`, "[0]"},
		// `(s)` is the group's other split, and it is a different rule:
		// measured, it splits the text whatever quoting it was written with.
		{"the group's own separator splits too", `unset u; : ${(As:,:)u=a,b}; printf "<%s>" "${u[@]}"`, "<a><b>"},
		{"and quoting does not protect there", `unset u; : ${(As:,:)u="a,b"}; printf "<%s>" "${u[@]}"`, "<a><b>"},
		// The rest of the group runs on what the expansion *yields* and
		// never reaches the store. These two are the discriminators: a
		// pipeline that stored its own output would upper-case and sort.
		{"a case conversion does not reach the store", `unset u; : ${(AU)=u=x y}; printf "<%s>" "${u[@]}"`, "<x><y>"},
		{"nor does an ordering", `unset u; : ${(Ao)=u=b a c}; printf "<%s>" "${u[@]}"`, "<b><a><c>"},
		{"nor a join", `unset u; a=(1 2); : ${(Aj:-:)u=${a[@]}}; printf "<%s>" "${u[@]}"`, "<1><2>"},
		// The operator is still the operator: `=` assigns only where the
		// name is unset, and `::=` always does.
		{"a set name is not assigned at all", `u=set; : ${(A)u=x y}; printf "[%s]" "$u"`, "[set]"},
		{"the colon form tests the value as well", `u=; : ${(A)=u:=x y}; printf "<%s>" "${u[@]}"`, "<x><y>"},
		{"and the always-assign assigns over one", `u=set; : ${(A)=u::=x y}; printf "<%s>" "${u[@]}"`, "<x><y>"},
		// What the expansion yields is the list, so the enclosing quoting
		// decides whether it is one word or several — exactly as for a plain
		// `${a[@]}`.
		{"the yield is the elements", `unset u; a=(1 2); printf "<%s>" ${(A)u=${a[@]}}`, "<1><2>"},
		{"and one word where it is quoted", `unset u; a=(1 2); printf "<%s>" "${(A)u=${a[@]}}"`, "<1 2>"},
		{"a scalar assignment of it joins", `unset u; x=${(A)=u=a b}; printf "[%s]" "$x"`, "[a b]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, arrayAssigning, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The `=` split reaches the store through an enclosing quote, which the
// operand's spans have inherited by the time anything looks at them.
//
// Its own row because it is the one place the span model has to be overridden
// rather than read: measured, `"${(A)=u=x y}"` stores two elements — the
// outer quotes not protecting the value — where `${(A)=u="x y"}` stores one.
// The two are indistinguishable at the span, so the override is armed from
// the enclosing quoting instead. See splitLiterals.evenQuoted.
func TestTheArrayFlagSplitsThroughAnEnclosingQuote(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the literal text still splits", `unset u; : "${(A)=u=x y}"; printf "<%s>" "${u[@]}"`, "<x><y>"},
		// And a parameter inside that quote does not, because a quoted
		// expansion is one field wherever it stands.
		{"a parameter inside it does not", `unset u; s="p q"; : "${(A)=u=$s}"; printf "<%s>" "${u[@]}"`, "<p q>"},
		// Nor a list, for the same reason and with the elements joined.
		{"nor does a list", `unset u; a=(1 2); : "${(A)=u=${a[*]}}"; printf "<%s>" "${u[@]}"`, "<1 2>"},
		{"and without the `=` nothing splits", `unset u; : "${(A)u=x y}"; printf "<%s>" "${u[@]}"`, "<x y>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, arrayAssigning, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// What is still refused is named, and the two refusals are separate
// measurements rather than one.
func TestTheArrayFlagRefusesWhatItDoesNotCarry(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// `(AA)` asks for an *association*, which needs the fields paired
		// off and a refusal of its own for an odd count.
		{"the doubled letter asks for a table", `: ${(AA)u=k v}`, "sh: ${(AA)u=k v}: the (AA) expansion flag is not implemented for an assignment\n"},
		// `(P)` moves the name along, and the target is a text that may
		// spell an element or a reference rather than a name.
		{"and a (P) beside it moves the name", `v=tgt; : ${(AP)v::=x y}`, "sh: ${(AP)v::=x y}: the (A) expansion flag is not implemented beside a (P) for an assignment\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, arrayAssigning, nil)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status = 0, want the assignment refused")
			}
		})
	}
}

// The operand is expanded **once**, which a command substitution in it is the
// only way to see.
//
// The array path builds fields where the scalar path builds text, so it is a
// second expansion of the same word if the branch is taken late — and a
// second run of `$(…)` is a side effect nobody asked for. Measured by
// counting the lines the substitution writes to the output.
func TestTheArrayFlagExpandsTheOperandOnce(t *testing.T) {
	const src = `unset u; : ${(A)=u=$(printf RAN; printf "m n")}; printf "<%s>" "${u[@]}"`
	out, st := runGrammar(t, src, arrayAssigning, nil)
	if want := "<RANm><n>"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
