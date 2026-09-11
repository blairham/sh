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
