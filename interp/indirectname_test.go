// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// indirectNameRun is flagsRun with the array grammar the indirection's base
// needs: a list to read a name out of the front of, a subscript on it, a
// comma read as a range, and an expansion standing where a name would.
//
// The axes are named rather than a shell. `(P)` beside an array base exists
// where arrays start at one, and a row reading `n[2]` of a two-element array
// would be about the base and not about the flag anywhere else.
func indirectNameRun(t *testing.T, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.ParamExpansionFlags = true
	d.NestedParamExpansion = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.SplitParamExpansion = No
	sem.GlobExpansionResults = No
	sem.FatalErrorStatusIsOne = Yes
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	sem.ArrayBaseIsZero = No
	sem.SubscriptCommaIsARange = Yes
	sem.ArrayLengthWithoutSubscriptIsCount = Yes
	// A bare name is the whole array, which is the dialect the flag belongs
	// to and is what makes `${(P)n}` over an association a *list* rather
	// than one element of it.
	sem.ArrayScalarIsTheWholeArray = Yes
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Dialect: &d, Semantics: &sem, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// The `(P)` flag reads a *name* out of the front of what its base came to.
//
// A base holding more than one word is not a name with a space in it — no
// such parameter can exist — so the words behind the first are dropped. This
// shell joined them and looked the join up, which found nothing: every base
// of more than one element substituted empty at status 0, and the control
// that hid it for so long is that a one-element base is the shape every test
// had (#1639, #1543).
//
// The rows about a *scalar* holding a space are what fix the reading. They
// answer the same as the array rows, so this is the front of the text and not
// the first element of a list — and it is why the two issues are one change.
//
// Named for the flag rather than for a shell, per the rule in AGENTS.md.
// Measured on zsh 5.9.2, the only shell in the panel with the flag; the same
// rows are in the corpus under
// `param/expansion-flags-indirection-names-the-front-of-its-base`.
func TestTheIndirectionFlagNamesTheFrontOfItsBase(t *testing.T) {
	// x and y are the parameters the bases below name, and n and s are two
	// spellings of the same base: a list of names and a string of them.
	const setup = `x=(p q); y=(r s); z=(t u); n=(x y z); s="x y"; `
	for _, tc := range []struct{ name, src, want string }{
		{
			"a list base resolves its first element",
			`printf "[%s]" "${(P)n}"`,
			"[p q]",
		},
		{
			"a scalar base of several words resolves the first of them",
			`printf "[%s]" "${(P)s}"`,
			"[p q]",
		},
		{
			"a subscript naming one element resolves that element",
			`printf "[%s]" "${(P)n[2]}"`,
			"[r s]",
		},
		{
			"a subscript naming one element that is not there is no name",
			`printf "[%s]" "${(P)n[4]}"`,
			"[]",
		},
		{
			"a whole-array subscript is not part of the resolution",
			`printf "[%s]" "${(P)n[@]}" "${(P)n[*]}"`,
			"[p q][p q]",
		},
		{
			"and neither is a range, wherever it starts",
			`printf "[%s]" "${(P)n[1,2]}" "${(P)n[2,3]}" "${(P)n[3,3]}"`,
			"[p q][p q][p q]",
		},
		{
			"any character a name cannot hold ends it",
			`v="x-y"; w="x=y"; printf "[%s]" "${(P)v}" "${(P)w}"`,
			"[p q][p q]",
		},
		{
			"a leading space is no name at all",
			`v=" x"; printf "[%s]" "${(P)v}"`,
			"[]",
		},
		{
			"an element that is empty is no name either",
			`e=("" x); printf "[%s]" "${(P)e}"`,
			"[]",
		},
		{
			"a subscript written in the resolved text is part of the name",
			`v="x[1] junk"; printf "[%s]" "${(P)v}"`,
			"[p]",
		},
		{
			"a digit run is a positional parameter",
			`set -- aa bb cc; v="2x"; printf "[%s]" "${(P)v}"`,
			"[bb]",
		},
		{
			"and a one-character special name is that one character",
			`set -- aa bb cc; v="#x"; w="@x"; printf "[%s]" "${(P)v}" "${(P)w}"`,
			"[3][aa bb cc]",
		},

		// The controls: a one-element base and a base naming nothing were
		// already right, so a case failing only above is a case about the
		// words behind the first and not about the flag.
		{
			"a one-element base is the shape that was already right",
			`one=(x); printf "[%s]" "${(P)one}"`,
			"[p q]",
		},
		{
			"a base naming no parameter is still empty",
			`v=nosuch; printf "[%s]" "${(P)v}"`,
			"[]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := indirectNameRun(t, setup+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%q: out=%q status=%d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// What a `(P)` came to keeps the shape of the parameter it landed on.
//
// The flag moves the expansion to a different parameter, so what is
// substituted is that parameter's list where it has one — and the subscript
// the *base* was written with does not travel with it. This shell had both
// halves backwards at once: it joined the result to one field, so anything
// counting or subscripting it counted characters, and it carried the base's
// `[@]` across, so a quoted expansion came back as one field per element of a
// parameter it was no longer about (#1638).
//
// Measured on zsh 5.9.2; the corpus row is
// `param/expansion-flags-indirection-keeps-the-resolved-shape`.
func TestTheIndirectionFlagKeepsTheResolvedShape(t *testing.T) {
	const setup = `arr=(x y z); na=arr; typeset -A tab; tab=(k1 v1); n=tab; typeset -A one; one=(a arr); `
	for _, tc := range []struct{ name, src, want string }{
		{
			"a length over the result counts the elements it landed on",
			`printf "[%s]" "${#${(P)na}}"`,
			"[3]",
		},
		{
			"an association counts its pairs",
			`printf "[%s]" "${#${(P)n}}"`,
			"[1]",
		},
		{
			"the letters rename, so the length is the renamed parameter's",
			`ARR=(q r); h=arr; printf "[%s]" "${#${(UP)h}}"`,
			"[2]",
		},
		{
			"a base reaching the name through a subscript counts the same",
			`printf "[%s]" "${#${(P)one[a]}}"`,
			"[3]",
		},
		{
			"the base's own whole-array subscript does not keep the fields",
			`printf "[%s]" "${(P)one[@]}"`,
			"[x y z]",
		},
		{
			"while the letter still does",
			`printf "[%s]" "${(@P)one[@]}"`,
			"[x][y][z]",
		},

		// The controls: the same lengths without the flag were already
		// right, and they are the answers this change must *not* give the
		// rows above.
		{
			"a nested expansion that is a value is measured as text",
			`printf "[%s]" "${#${arr}}" "${#${(k)tab}}"`,
			"[5][2]",
		},
		{
			"and the fields-keeping flag on a value is the count",
			`printf "[%s]" "${#${(@)arr}}" "${#arr}"`,
			"[3][3]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := indirectNameRun(t, setup+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%q: out=%q status=%d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// referenceRun is indirectNameRun with the two grammars a *reference* needs
// beyond a plain name: a flag group inside a subscript, and a subscript read
// over a scalar as a character.
//
// Named for the constructs, not for a shell. Both exist wherever `(P)` does,
// which is what makes them the right grammar for these rows rather than an
// extra this file's other tests happen not to need.
func referenceRun(t *testing.T, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.ParamExpansionFlags = true
	d.NestedParamExpansion = true
	d.ArraySubscriptFlags = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.SplitParamExpansion = No
	sem.GlobExpansionResults = No
	sem.GlobNoMatchIsError = Yes
	sem.FatalErrorStatusIsOne = Yes
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	sem.ArrayBaseIsZero = No
	sem.SubscriptCommaIsARange = Yes
	sem.ArrayLengthWithoutSubscriptIsCount = Yes
	sem.ArrayScalarIsTheWholeArray = Yes
	sem.ScalarSubscriptIsACharacter = Yes
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Dialect: &d, Semantics: &sem, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// The resolved text is a parameter **reference** and not only a name, so
// every subscript this shell answers on a name it answers here.
//
// It was taken apart by hand into a name plus one arithmetic index, and
// everything else fell through to the plain-name lookup and found nothing —
// empty, at status 0, which is a plausible value for a real element. The
// first row of each pair is what moved; the `[1]` row is the control that
// already worked and is what says the shape was the problem and not the
// indirection (#1852).
//
// Measured on zsh 5.9.2, 2026-09-12.
func TestTheIndirectionReadsItsTextAsAReference(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a whole-array subscript", `x=(p q); v='x[@]'; printf "[%s]" "${(P)v}"`, "[p][q]"},
		{"and the joining spelling", `x=(p q); v='x[*]'; printf "[%s]" "${(P)v}"`, "[p q]"},
		{"one element, the control", `x=(p q); v='x[1]'; printf "[%s]" "${(P)v}"`, "[p]"},
		{"a range", `x=(p q r); v='x[1,2]'; printf "[%s]" "${(P)v}"`, "[p q]"},
		{"a character of a scalar", `s=abc; v='s[2]'; printf "[%s]" "${(P)v}"`, "[b]"},
		{"and a range of one", `s=abcdef; v='s[2,4]'; printf "[%s]" "${(P)v}"`, "[bcd]"},
		{"a search", `a=(p q); v='a[(r)q]'; printf "[%s]" "${(P)v}"`, "[q]"},
		{"and its index form", `a=(p q); v='a[(i)q]'; printf "[%s]" "${(P)v}"`, "[2]"},
		{
			// The subscript is live text and not a literal, which is what
			// the parse buys: a substitution written into a resolved
			// reference is performed when the reference is read.
			"a substitution inside the subscript",
			`x=(p q r); i=2; v='x[$i]'; printf "[%s]" "${(P)v}"`, "[q]",
		},
		{
			"and one inside a search's operand",
			`a=(p q); w=q; v='a[(r)$w]'; printf "[%s]" "${(P)v}"`, "[q]",
		},
		{
			// The whole-array subscript makes a *list*, which is the half a
			// join would hide: three fields, not one word of three.
			"a whole-array reference keeps its fields",
			`x=(p q r); v='x[@]'; set -- ${(P)v}; printf "[n=%s]" "$#"`, "[n=3]",
		},
		{
			// The first row keeps its two fields **in quotes**, which is
			// the written `@` and not the list-ness. This is the sibling
			// that says so: a reference to the array itself joins, as the
			// `[*]` and range rows above already do.
			"a reference to the array itself joins in quotes",
			`x=(p q); n=(x y); printf "[%s]" "${(P)n}"`, "[p q]",
		},
		{
			// `k` and `v` are the letters the *second* lookup answers, and
			// a reference is that lookup — so they have to reach it. Over
			// an ordinary array `k` is the index the subscript named.
			"the key letter reaches the reference",
			`x=(p q); v='x[2]'; printf "[%s]" "${(kP)v}"`, "[2]",
		},
		{
			"and over a table it is the key",
			`typeset -A tab=(k1 v1); v='tab[k1]'; printf "[%s]" "${(kP)v}" "${(P)v}"`, "[k1][v1]",
		},
		{
			"over a whole table it is the keys",
			`typeset -A tab=(k1 v1); v='tab[@]'; printf "[%s]" "${(kP)v}" "${(vP)v}"`, "[k1][v1]",
		},
		{
			// The rest of the group is not the lookup's: it acts on what
			// came out, once.
			"and the other letters still act on the result",
			`x=(p q); v='x[2]'; printf "[%s]" "${(UP)v}"`, "[Q]",
		},
		{
			"and its length is the element count",
			`x=(p q r); v='x[@]'; printf "[%s]" "${#${(P)v}}"`, "[3]",
		},
		{
			// The nested subscript reaches the same reference: the text's
			// own subscript is the link *before* the outer one rather than
			// something it replaces.
			"a subscript on a reference reads what it named",
			`x=(p q r); v='x[@]'; printf "[%s]" "${${(P)v}[2]}"`, "[q]",
		},
		{
			"and the length of a reference naming one element is a width",
			`x=(p q r); v='x[1]'; printf "[%s]" "${#${(P)v}}"`, "[1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := referenceRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// An element that is not there is still *unset*, which is what keeps the
// reference reading from making every miss look like an empty value.
func TestAReferenceThatNamesNothingIsUnset(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an index past the last element", `x=(p q); v='x[9]'; printf "[%s]" "${(P)v-D}"`, "[D]"},
		{"an absent key", `typeset -A m=(k v1); v='m[zz]'; printf "[%s]" "${(P)v-D}"`, "[D]"},
		{"where a key that is there is set", `typeset -A m=(k v1); v='m[k]'; printf "[%s]" "${(P)v-D}"`, "[v1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := referenceRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The base's *own* subscript has one index that is not read as an index: the
// one before the first, which no element has. Every other subscript naming
// nothing is no name at all, and this one resolves the base's first element
// as though none had been written.
//
// A corner no script can depend on, reproduced rather than left because the
// alternative is a plausible empty at status 0 (#1852).
func TestTheIndexNoElementHasResolvesTheWholeBase(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the index before the first", `n=(x y z); x=(p q); printf "[%s]" "${(P)n[0]}"`, "[p q]"},
		{"which is the value and not the numeral", `n=(x y z); x=(p q); printf "[%s]" "${(P)n[1-1]}"`, "[p q]"},
		{"an index past the last is no name", `n=(x y z); x=(p q); printf "[%s]" "${(P)n[4]}"`, "[]"},
		{"and one below the first is no name either", `n=(x y z); x=(p q); printf "[%s]" "${(P)n[-4]}"`, "[]"},
		{
			// An association's `[0]` is a key like any other, so the corner
			// is the indexed array's alone.
			"a table's key of that spelling is a key",
			`typeset -A nt=(a tab); typeset -A tab=(k1 v1); printf "[%s]" "${(P)nt[0]}"`, "[]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := referenceRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
