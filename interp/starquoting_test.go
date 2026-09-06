// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Under a `[*]` subscript, *quoting* is what decides whether an operator sees
// the list or the joined string, and both halves were decided without looking
// at it.
//
// The rule, measured across the family: quotes join first, `[*]` joins last.
// A quoted expansion hands the operator one string; an unquoted `[*]` hands
// it the elements and joins whatever comes out.
//
// Measured 2026-09-06 against bash 5.3.15, bash 3.2.57, that build invoked as
// `sh`, dash, ksh93u+ 2012-08-01 and zsh 5.9.2.
//
//	a=(oxo yo); printf "[%s]" ${a[*]%o}     [ox][y]  in all four with arrays
//	a=(oxo yo); printf "[%s]" "${a[*]%o}"   [ox y]   bash, ksh93
//	                                        [oxo y]  zsh
//
// So the *unquoted* spelling has one answer nobody has to be asked for, and
// the axis is a question about the quoted one alone.

// starQuoteRun answers the axes these rows reach and the grammar flag the
// element-selecting operators need.
func starQuoteRun(t *testing.T, src string, distributes Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamElementSelection = true
	}, func(r *Runner) {
		sem := CoreSemantics()
		sem.ArrayScalarIsTheWholeArray = No
		sem.OperatorDistributesOverStarSubscript = distributes
		r.Semantics = &sem
	})
}

// An unquoted `${a[*]%o}` distributes whichever way the axis is answered —
// including not at all. The bug gave it the quoted reading, so in the dialect
// that answers no it came back as one field holding `oxo y`: the trim applied
// to a field boundary instead of to an element.
func TestAnUnquotedStarSubscriptAlwaysDistributes(t *testing.T) {
	const src = `a=(oxo yo); set -- ${a[*]%o}; printf "%d" "$#"; printf "[%s]" "$@"`
	for _, a := range []Answer{Yes, No, Unspecified} {
		if out, st := starQuoteRun(t, src, a); out != `2[ox][y]` || st != 0 {
			t.Errorf("with the axis %v: got %q status %d, want %q at 0", a, out, st, `2[ox][y]`)
		}
	}
}

// The quoted spelling is the axis, and both of its answers are one field —
// the same characters differently trimmed, which is why the text is the
// assertion here and the count is the assertion above.
func TestAQuotedStarSubscriptAsksWhetherTheOperatorDistributes(t *testing.T) {
	const src = `a=(oxo yo); set -- "${a[*]%o}"; printf "%d" "$#"; printf "[%s]" "$@"`
	if out, st := starQuoteRun(t, src, Yes); out != `1[ox y]` || st != 0 {
		t.Errorf("distributing: got %q status %d, want %q at 0", out, st, `1[ox y]`)
	}
	if out, st := starQuoteRun(t, src, No); out != `1[oxo y]` || st != 0 {
		t.Errorf("joining first: got %q status %d, want %q at 0", out, st, `1[oxo y]`)
	}
}

// The `[@]` spelling is neither question: it keeps one field per element and
// the operator reaches every one of them, whatever the axis says.
func TestAnAtSubscriptIsNotTheDistributionQuestion(t *testing.T) {
	for _, src := range []string{
		`a=(oxo yo); set -- ${a[@]%o}; printf "%d" "$#"; printf "[%s]" "$@"`,
		`a=(oxo yo); set -- "${a[@]%o}"; printf "%d" "$#"; printf "[%s]" "$@"`,
	} {
		for _, a := range []Answer{Yes, No, Unspecified} {
			if out, st := starQuoteRun(t, src, a); out != `2[ox][y]` || st != 0 {
				t.Errorf("%s with the axis %v: got %q status %d, want %q at 0", src, a, out, st, `2[ox][y]`)
			}
		}
	}
}

// The element-selecting three are the silent direction: quoted, the array
// joins first and the operator tests that one value, so a pattern matching
// some elements drops *nothing* — and a filter that ran where the shell would
// have left the array alone looks exactly like a pattern that did not match.
func TestAQuotedStarSubscriptDoesNotFilterElements(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// `ba*` matches two of three elements and does not match the
			// joined `foo bar baz`, so filtering leaves one field and the
			// shell leaves all three joined.
			"exclusion by pattern",
			`a=(foo bar baz); set -- "${a[*]:#ba*}"; printf "%d" "$#"; printf "[%s]" "$@"`,
			`1[foo bar baz]`,
		},
		{
			"set difference",
			`a=(x y z); b=(y); set -- "${a[*]:|b}"; printf "%d" "$#"; printf "[%s]" "$@"`,
			`1[x y z]`,
		},
		{
			// The intersection is the direction that empties it, and a
			// dropped value is one *empty* field rather than no field —
			// quoting is the whole guarantee, and it does not depend on what
			// the operator left.
			"set intersection",
			`a=(x y z); b=(nope); set -- "${a[*]:*b}"; printf "%d" "$#"; printf "[%s]" "$@"`,
			`1[]`,
		},
		{
			// The value the operator tests is the elements joined on the
			// first character of IFS, not on a space: the pattern matches
			// only when it is written with the separator that is actually
			// there.
			"the subject is joined on IFS",
			`IFS=-; a=(oxo yo); set -- "${a[*]:#ox?-yo}"; printf "%d" "$#"; printf "[%s]" "$@"`,
			`1[]`,
		},
	} {
		if out, st := starQuoteRun(t, c.src, No); out != c.want || st != 0 {
			t.Errorf("%s: got %q status %d, want %q at 0", c.name, out, st, c.want)
		}
	}
}

// Unquoted, the same three still filter — the half that was already right and
// must stay so. Answered three ways, because this is not the axis.
func TestAnUnquotedStarSubscriptStillFiltersElements(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(foo bar baz); set -- ${a[*]:#ba*}; printf "%d" "$#"; printf "[%s]" "$@"`, `1[foo]`},
		{`a=(x y z); b=(y); set -- ${a[*]:|b}; printf "%d" "$#"; printf "[%s]" "$@"`, `2[x][z]`},
		// `printf "[%s]"` runs its format once with no arguments, so the
		// empty brackets are printf's and the `0` is the field count — the
		// reason the count is printed at all.
		{`a=(x y z); b=(nope); set -- ${a[*]:*b}; printf "%d" "$#"; printf "[%s]" "$@"`, `0[]`},
	} {
		for _, a := range []Answer{Yes, No, Unspecified} {
			if out, st := starQuoteRun(t, c.src, a); out != c.want || st != 0 {
				t.Errorf("%s with the axis %v: got %q status %d, want %q at 0", c.src, a, out, st, c.want)
			}
		}
	}
}

// The slice is the exception that proves the rule is about the *operator* and
// not about quoting alone: under `[*]` its offset counts elements whichever
// way the expansion is quoted, and only the join afterwards differs. Unanimous
// in all three shells with arrays, so it is core and reaches no axis.
func TestASliceUnderAStarSubscriptAlwaysReadsTheList(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(one two three); set -- ${a[*]:1}; printf "%d" "$#"; printf "[%s]" "$@"`, `2[two][three]`},
		{`a=(one two three); set -- "${a[*]:1}"; printf "%d" "$#"; printf "[%s]" "$@"`, `1[two three]`},
		{`a=(one two three); set -- "${a[*]:1:1}"; printf "%d" "$#"; printf "[%s]" "$@"`, `1[two]`},
	} {
		for _, a := range []Answer{Yes, No, Unspecified} {
			if out, st := starQuoteRun(t, c.src, a); out != c.want || st != 0 {
				t.Errorf("%s with the axis %v: got %q status %d, want %q at 0", c.src, a, out, st, c.want)
			}
		}
	}
}

// The quoted bare name is the same rule reached through `$a`, and it comes for
// free once `[*]` is right: where the name is the whole array, `"${a:1}"` is
// the list sliced and then joined.
//
// Only the slice needs the rewrite. Every other operator is already right
// through the scalar path, because the quoted value *is* the elements joined
// and an operator applied to that one string is the "quotes join first"
// reading — which is what the second table asserts, with the rewrite's axis
// answered both ways.
func TestAQuotedBareArrayNameSlicesTheList(t *testing.T) {
	whole := func(t *testing.T, src string, scalarIsArray Answer) (string, int) {
		t.Helper()
		return runGrammar(t, src, func(d *syntax.Dialect) {
			d.ParamElementSelection = true
		}, func(r *Runner) {
			sem := CoreSemantics()
			sem.ArrayScalarIsTheWholeArray = scalarIsArray
			sem.OperatorDistributesOverStarSubscript = No
			r.Semantics = &sem
		})
	}
	for _, c := range []struct{ src, isArray, isElement string }{
		{`a=(one two three); printf "[%s]" "${a:1}"`, `[two three]`, `[ne]`},
		{`a=(one two three); printf "[%s]" "${a:1:1}"`, `[two]`, `[n]`},
		{`IFS=-; a=(one two three); printf "[%s]" "${a:1}"`, `[two-three]`, `[ne]`},
		// One element is enough for the two readings to diverge, which is why
		// the slice is the operator that must not be waved through at n=1:
		// dropping the only element leaves nothing, where dropping the first
		// character leaves three.
		{`a=(solo); printf "[%s]" "${a:1}"`, `[]`, `[olo]`},
	} {
		if out, st := whole(t, c.src, Yes); out != c.isArray || st != 0 {
			t.Errorf("%s where the name is the array: got %q status %d, want %q at 0", c.src, out, st, c.isArray)
		}
		if out, st := whole(t, c.src, No); out != c.isElement || st != 0 {
			t.Errorf("%s where the name is one element: got %q status %d, want %q at 0", c.src, out, st, c.isElement)
		}
	}
	// The operators the rewrite deliberately leaves on the scalar path. Both
	// answers of the axis, because the reading must not depend on the route.
	for _, c := range []struct{ src, isArray, isElement string }{
		{`a=(oxo yo); printf "[%s]" "${a%o}"`, `[oxo y]`, `[ox]`},
		{`a=(oxo yo); printf "[%s]" "${a#o}"`, `[xo yo]`, `[xo]`},
		{`a=(oxo yo); printf "[%s]" "${a:#ox*}"`, `[]`, `[]`},
		{`a=(oxo yo); b=(yo); printf "[%s]" "${a:|b}"`, `[oxo yo]`, `[oxo]`},
		{`a=(oxo yo); printf "[%s]" "${a:-d}"`, `[oxo yo]`, `[oxo]`},
		{`a=(oxo yo); printf "[%s]" "${a}"`, `[oxo yo]`, `[oxo]`},
	} {
		if out, st := whole(t, c.src, Yes); out != c.isArray || st != 0 {
			t.Errorf("%s where the name is the array: got %q status %d, want %q at 0", c.src, out, st, c.isArray)
		}
		if out, st := whole(t, c.src, No); out != c.isElement || st != 0 {
			t.Errorf("%s where the name is one element: got %q status %d, want %q at 0", c.src, out, st, c.isElement)
		}
	}
}

// A scalar and an empty array are not this question at all: neither has a
// reading the two answers differ on, so the rewrite must decline and the
// substring must stay a substring.
func TestTheQuotedRewriteDeclinesWhereThereIsNoList(t *testing.T) {
	run := func(t *testing.T, src string, a Answer) (string, int) {
		t.Helper()
		return runGrammar(t, src, nil, func(r *Runner) {
			sem := CoreSemantics()
			sem.ArrayScalarIsTheWholeArray = a
			r.Semantics = &sem
		})
	}
	// A scalar asks nothing at all, however the axis is answered — including
	// not at all, where an unanswered axis is a refusal.
	for _, a := range []Answer{Yes, No, Unspecified} {
		if out, st := run(t, `v=plain; printf "[%s]" "${v:1}"`, a); out != `[lain]` || st != 0 {
			t.Errorf("a scalar with the axis %v: got %q status %d, want %q at 0", a, out, st, `[lain]`)
		}
	}
	// An empty array is one field holding nothing under either reading —
	// there is no list to slice and no element to take characters from — so
	// the rewrite must decline and the answer must not depend on the axis.
	//
	// Only Yes and No. An empty array's bare name reaches
	// ArrayScalarIsTheWholeArray on the scalar path since #1049, so an
	// unanswered axis is refused there rather than here: `a=(); echo "${a}"`
	// with no operator at all is refused on a bare core too, which is what
	// says the refusal is not this rewrite's.
	for _, a := range []Answer{Yes, No} {
		if out, st := run(t, `a=(); printf "[%s]" "${a:1}"`, a); out != `[]` || st != 0 {
			t.Errorf("an empty array with the axis %v: got %q status %d, want %q at 0", a, out, st, `[]`)
		}
	}
}
