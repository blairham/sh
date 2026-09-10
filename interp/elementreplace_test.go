// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// wholeElementReplacing turns on the grammar these tests need, by the
// constructs' names rather than by a shell's.
func wholeElementReplacing(d *syntax.Dialect) {
	d.ParamWholeElementReplace = true
	d.ParamExpansionFlags = true
	d.ArrayLiteral = true
	d.ArraySubscript = true
}

// `${a:/pattern/replacement}` replaces the elements the pattern matches
// **whole**, through every spelling that reaches a list.
//
// One test over the routes because they are three ways into one operator and
// the way to get this wrong is to implement it on one of them.
func TestWholeElementReplacementReplacesMatchingElements(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"through a flag group",
			`a=(foo bar); printf "[%s]" "${(@)a:/foo/Z}"`,
			"[Z][bar]",
		},
		{
			"through an [@] subscript",
			`a=(foo bar); printf "[%s]" "${a[@]:/foo/Z}"`,
			"[Z][bar]",
		},
		{
			"a pattern with a metacharacter takes every element it matches",
			`a=(foo bar baz); printf "[%s]" "${(@)a:/ba*/Z}"`,
			"[foo][Z][Z]",
		},
		{
			// The whole point of the operator, and the row that separates it
			// from the span replacement: `foo` is a prefix of `foobar` and
			// the element is left alone anyway.
			"a pattern matching only part of an element replaces nothing",
			`v=foobar; printf "[%s]" "${v:/foo/Z}"`,
			"[foobar]",
		},
		{
			"the same characters as a span replacement do replace the part",
			`v=foobar; printf "[%s]" "${v/foo/Z}"`,
			"[Zbar]",
		},
		{
			"a scalar the pattern matches whole becomes the replacement",
			`v=foo; printf "[%s]" "${v:/foo/Z}"`,
			"[Z]",
		},
		{
			"the replacement holds every slash after the first",
			`v=foo; printf "[%s]" "${v:/foo/a/b}"`,
			"[a/b]",
		},
		{
			"matching is case-sensitive",
			`a=(Foo bar); printf "[%s]" "${(@)a:/foo/Z}"`,
			"[Foo][bar]",
		},
		{
			// An empty pattern is not "no pattern" and not `*`: it matches
			// the empty string and only that, so the empty element goes and
			// the other stays.
			"an empty pattern takes the empty element and no other",
			`a=("" one); printf "[%s]" "${(@)a:/}"`,
			"[one]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runGrammar(t, tc.src, wholeElementReplacing, nil)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0", status)
			}
		})
	}
}

// A value the pattern does not match is left exactly as it was, silently, on a
// scalar as well as on a list.
//
// The half a fix that only handles the matching case gets wrong. It looks
// right in a demo and it is what three real startup files spend most of their
// calls on, so it is asserted on its own and with the status.
func TestWholeElementReplacementLeavesUnmatchedValuesAlone(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a scalar full of slashes is not an arithmetic offset",
			`v=/a/b/c; printf "[%s]" "${v:/b/Z}"`,
			"[/a/b/c]",
		},
		{
			"and not a pattern that matched either",
			`v=/a/b/c; printf "[%s]" "${v:/b}"`,
			"[/a/b/c]",
		},
		{
			"a pattern nothing matches leaves every element",
			`a=(foo bar); printf "[%s]" "${(@)a:/zzz/Z}"`,
			"[foo][bar]",
		},
		{
			"an empty pattern against a value that is not empty",
			`v=abcdef; printf "[%s]" "${v:/}"`,
			"[abcdef]",
		},
		{
			"an empty array stays empty",
			`a=(); set -- "${(@)a:/x/Y}"; printf "n=%d" "$#"`,
			"n=0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runGrammar(t, tc.src, wholeElementReplacing, nil)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0", status)
			}
		})
	}
}

// An element replaced by *nothing* is dropped from a list, where a scalar
// keeps its emptied value.
//
// Counted rather than printed, because `printf` runs its format once even with
// nothing to fill it: `[]` comes out both from an empty list and from a list
// holding one empty string, and those are different answers here.
func TestWholeElementReplacementDropsWhatItEmpties(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an empty replacement removes the elements it matched",
			`a=(foo bar baz); set -- "${(@)a:/b*/}"; printf "n=%d" "$#"; printf "[%s]" "$@"`,
			"n=1[foo]",
		},
		{
			"and omitting the slash says the same thing",
			`a=(foo bar baz); set -- "${(@)a:/b*}"; printf "n=%d" "$#"; printf "[%s]" "$@"`,
			"n=1[foo]",
		},
		{
			"a replacement that expands to nothing removes them too",
			`e=; a=(foo bar); set -- "${(@)a:/foo/$e}"; printf "n=%d" "$#"; printf "[%s]" "$@"`,
			"n=1[bar]",
		},
		{
			"removal is in place, so what is left keeps its order",
			`a=(x foo y); set -- "${(@)a:/foo/}"; printf "n=%d" "$#"; printf "[%s]" "$@"`,
			"n=2[x][y]",
		},
		{
			"a list every element matches is no field at all",
			`a=(foo bar); set -- "${(@)a:/*/}"; printf "n=%d" "$#"`,
			"n=0",
		},
		{
			// The row that says it is the *replacement* being empty that
			// drops an element and not emptiness in the result: this element
			// was already empty, did not match, and is still there.
			"an element that was already empty and did not match is kept",
			`a=(foo "" bar); set -- "${(@)a:/x/Y}"; printf "n=%d" "$#"`,
			"n=3",
		},
		{
			// A scalar is not a list. The same operator over one value leaves
			// one empty field where a list would have none.
			"a scalar emptied by the operator is still a field",
			`v=foo; set -- "${v:/foo}"; printf "n=%d" "$#"; printf "[%s]" "$@"`,
			"n=1[]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runGrammar(t, tc.src, wholeElementReplacing, nil)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0", status)
			}
		})
	}
}

// Quoting decides what the operator is even looking at, and the rule is the
// element filters' own: quotes join first and `[*]` joins last.
func TestWholeElementReplacementFollowsTheJoiningRule(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a quoted [@] is one field per element",
			`a=(foo bar baz); printf "[%s]" "${a[@]:/ba*/Z}"`,
			"[foo][Z][Z]",
		},
		{
			// The silent direction: a replacement that ran where the shell
			// hands the operator the joined string and it matches nothing.
			"a quoted [*] joins first, so the pattern is tested against the join",
			`a=(foo bar baz); printf "[%s]" "${a[*]:/ba*/Z}"`,
			"[foo bar baz]",
		},
		{
			"and unquoted it distributes",
			`a=(foo bar baz); printf "[%s]" ${a[*]:/ba*/Z}`,
			"[foo][Z][Z]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runGrammar(t, tc.src, wholeElementReplacing, nil)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0", status)
			}
		})
	}
}

// The operator does not write to the parameter — it substitutes and leaves the
// array as it found it.
func TestWholeElementReplacementLeavesTheParameterAlone(t *testing.T) {
	const src = `a=(foo bar); printf "[%s]" "${(@)a:/foo/Z}"; printf "|"; printf "[%s]" "${(@)a}"`
	out, status := runGrammar(t, src, wholeElementReplacing, nil)
	if want := "[Z][bar]|[foo][bar]"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if status != 0 {
		t.Errorf("status = %d, want 0", status)
	}
}

// The pattern operand is expanded once for the whole list, as every other
// operator inside `${ }` is: a command substitution in it runs a single time.
func TestWholeElementReplacementExpandsThePatternOnce(t *testing.T) {
	const src = `a=(foo bar baz); printf "[%s]" "${(@)a:/$(printf ran >&2; printf 'ba*')/Z}"`
	out, status := runGrammar(t, src, wholeElementReplacing, nil)
	if want := "ran[foo][Z][Z]"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if status != 0 {
		t.Errorf("status = %d, want 0", status)
	}
}

// The replacement is read **once** for the whole operator when the pattern
// reports nothing, and **again for each match** when it does — the rule the
// span replacement already follows, and the reason `$MATCH` is live here.
//
// The two halves are asserted from one shape apiece, because the difference is
// visible rather than a saving: a counter in the replacement counts only under
// a reporting pattern.
func TestWholeElementReplacementReadsTheReplacementPerReportingMatch(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		extended  bool
		want      string
	}{
		{
			"a pattern that reports nothing reads the replacement once",
			`a=(x y z); i=0; printf "[%s]" "${(@)a:/*/$((++i))}"`,
			true, "[1][1][1]",
		},
		{
			"a (#m) pattern reads it again for every element",
			`a=(x y z); i=0; printf "[%s]" "${(@)a:/(#m)*/$((++i))}"`,
			true, "[1][2][3]",
		},
		{
			"and $MATCH is the element the pattern took",
			`a=(foo bar); printf "[%s]" "${(@)a:/(#m)*/<$MATCH>}"`,
			true, "[<foo>][<bar>]",
		},
		{
			// The *positions* a report carries are counted from the array
			// base, which is an axis rather than this operator's, so what
			// they are is asserted where a dialect has answered it.
			"$MBEGIN and $MEND come with it",
			`a=(foo); printf "[%s]" "${(@)a:/(#m)*/$MBEGIN-$MEND}"`,
			true, "[0-2]",
		},
		{
			// Without the operators there is no `(#m)` to report: the same
			// line reads the flag group as an ordinary pattern, matches
			// nothing, and leaves the array alone. So the reporting is the
			// pattern's and not the operator's.
			"without the extended operators the same pattern reports nothing",
			`a=(foo bar); printf "[%s]" "${(@)a:/(#m)*/<$MATCH>}"`,
			false, "[foo][bar]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enable := func(d *syntax.Dialect) {
				wholeElementReplacing(d)
				d.PatternAlternation = true
				d.GlobQualifiers = true
			}
			out, status := runGrammar(t, tc.src, enable, func(r *Runner) {
				d := syntax.Core()
				enable(&d)
				r.Dialect = &d
				r.SetMatchOption(ExtendedPatternOperators, tc.extended)
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0", status)
			}
		})
	}
}
