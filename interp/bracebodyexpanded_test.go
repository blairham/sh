// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// braceBody runs one snippet under a vector that answers
// BraceBodyReadAfterExpansion the given way, with the other brace axes the
// rows reach pinned so that nothing here is decided by a refusal.
//
// The axes beside the subject are the ones every row would otherwise put:
// a range read from produced text asks BraceRangeEndpointsExpanded, and a
// counted range asks what its elements re-enter the word as. They are set to
// one reading rather than left unanswered, because a test whose rows are
// refused by name proves nothing about the axis it is named for.
func braceBody(t *testing.T, src string, read Answer) (string, int) {
	t.Helper()
	out, st := run(t, src+"\n", func(r *Runner) {
		s := *r.Semantics
		s.BraceBodyReadAfterExpansion = read
		s.BraceRangeEndpointsExpanded = Yes
		s.BraceOutputRereadAsText = No
		s.BraceRescanEntersFailedGroup = No
		// A produced body can hold an alternative that comes to nothing,
		// which is its own axis and not this one's: pinned to the reading
		// that removes it so that the rows here measure where the commas
		// were read and not what an empty alternative leaves.
		s.BraceEmptyAlternativeIsAField = No
		// The redirection row reaches the target's own two axes, which are
		// not this one's and must not be the thing that refuses it.
		s.RedirectTargetIsAnOrdinaryWord = No
		s.RedirectTargetTakesPathnameExpansion = No
		s.RedirectsUseEveryTarget = No
		r.Semantics = &s
	})
	return strings.TrimSpace(out), st
}

// A brace group's body is read after the expansions written in it, so a comma
// an expansion produced separates alternatives. See
// [Semantics.BraceBodyReadAfterExpansion] for the panel.
//
// Every row is stated twice — once under each answer — because the axis is
// about *which* reading applies and a row asserted only under the reading it
// was written for cannot tell a working switch from a constant.
func TestABraceBodyIsReadAfterTheExpansionsInIt(t *testing.T) {
	for _, tc := range []struct{ name, src, read, written string }{
		{
			"a produced comma separates alternatives",
			`e=a,b; echo {$e}`, "a b", "{a,b}",
		},
		{
			"and a written comma does not decide it",
			`e=a,b; echo {x,$e}`, "x a b", "x a,b",
		},
		{
			"the text on either side of the group comes with each name",
			`e=a,b; echo pre{$e}post`, "preapost prebpost", "pre{a,b}post",
		},
		{
			"a produced range is read as one",
			`e=1..3; echo {$e}`, "1 2 3", "{1..3}",
		},
		{
			"a produced comma beats a produced range",
			`e=1..3,z; echo {$e}`, "1..3 z", "{1..3,z}",
		},
		{
			"a comma an expansion produced anywhere in the body",
			`e=,; echo {a${e}b}`, "a b", "{a,b}",
		},
		{
			"quoting the expansion does not hide the comma it produced",
			`e=a,b; echo {"$e"}`, "a b", "{a,b}",
		},
		{
			"and quoting the word hides the braces under both readings",
			`e=a,b; echo "{$e}"`, "{a,b}", "{a,b}",
		},
		{
			"a produced brace closes no group",
			`e=}; echo {a,b$e`, "{a,b}", "{a,b}",
		},
		{
			"a written group nested in the body is still a group",
			`e=a,b; echo {{$e},z}`, "a b z", "{a,b} z",
		},
		{
			"a written group nested in an alternative too",
			`e=a,b; echo {x{p,q}$e}`, "xpa xqa b", "{x{p,q}a,b}",
		},
		{
			"a group behind the closing brace is still a factor",
			`e=a,b; echo {$e}{c,d}`, "ac ad bc bd", "{a,b}c {a,b}d",
		},
		{
			"and two produced groups multiply",
			`e=a,b; echo x{$e}y{$e}z`, "xayaz xaybz xbyaz xbybz", "x{a,b}y{a,b}z",
		},
		{
			// The empty alternative between the two commas is dropped here
			// under both readings, because BraceEmptyAlternativeIsAField is
			// pinned above to the reading that removes it. That is a
			// question of its own and not this axis's.
			"two produced commas make three alternatives",
			`e=a,,b; echo {$e}`, "a b", "{a,,b}",
		},
		{
			"a body that is neither list nor range keeps its braces",
			`e=a; echo {$e}`, "{a}", "{a}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := braceBody(t, tc.src, Yes)
			if st != 0 {
				t.Fatalf("status = %d, want 0; out = %q", st, got)
			}
			if got != tc.read {
				t.Errorf("read after expansion: %s = %q, want %q", tc.src, got, tc.read)
			}
			got, st = braceBody(t, tc.src, No)
			if st != 0 {
				t.Fatalf("status = %d, want 0; out = %q", st, got)
			}
			if got != tc.written {
				t.Errorf("read as written: %s = %q, want %q", tc.src, got, tc.written)
			}
		})
	}
}

// What a produced alternative leaves is inert: neither split, nor matched
// against the filesystem, nor expanded again.
//
// The written half of each pair is the control, and it is the half that makes
// the row mean anything — a shell that had simply stopped globbing would pass
// every "not matched" row on its own.
func TestAProducedBraceAlternativeIsInert(t *testing.T) {
	// `aa` and `ab` are what a pattern here has to match, so a row asserting
	// that a produced `a*` did *not* match is asserting against something.
	const fixture = `: > aa; : > ab; : > zz; `
	for _, tc := range []struct{ name, src, want string }{
		{
			"a produced pattern is not matched",
			`e='a*,z'; echo {$e}`, "a* z",
		},
		{
			"where the same pattern written is",
			`echo {a*,z}`, "aa ab z",
		},
		{
			"a written character behind a produced one still matches",
			`e=a; echo {p,$e*}`, "p aa ab",
		},
		{
			"and a produced one behind a written one does not",
			`e='*'; echo {p,a$e}`, "p a*",
		},
		{
			"a produced blank is no field separator",
			`e='a b,c'; f(){ echo $#; }; f {$e}`, "2",
		},
		{
			"a produced expansion is not expanded again",
			`x=BOOM; e='$x,b'; echo {$e}`, "$x b",
		},
		{
			"a body that produced no list is not split either",
			`e='a b'; echo {$e}`, "{a b}",
		},
		{
			"nor matched",
			`e='*'; echo {$e}`, "{*}",
		},
		{
			"while the word behind the group still matches",
			`e=a; echo {$e,b}*`, "aa ab b*",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := braceBody(t, fixture+tc.src, Yes)
			if st != 0 {
				t.Fatalf("status = %d, want 0; out = %q", st, got)
			}
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// A `{` an expansion produced leaves the group exactly as it was written, and
// ends the word's scan with it.
//
// The rows are producedBraceAbandonsTheGroup's own table. The `}` row is the
// discriminator that says it is the character and not the balance, and the
// two mirror rows say the note is on the body and on nothing else.
func TestAProducedOpeningBraceLeavesTheGroupAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a produced closing brace is a character", `e=}; echo {a,b$e,c}`, "a b} c"},
		{"a produced opening brace is not", `e={; echo {a,b$e,c}`, "{a,b{,c}"},
		{"balanced, and still not", `e={z}; echo {a,b$e,c}`, "{a,b{z},c}"},
		{"a produced list is not one either", `e={z,y}; echo {a,b$e,c}`, "{a,b{z,y},c}"},
		{"nor the pair written the other way round", `e=}x{; echo {a,b$e,c}`, "{a,b}x{,c}"},
		{"in an alternative of its own", `e=a{b; echo {x,$e}`, "{x,a{b}"},
		{"as the whole body", `e={z,y}; echo {$e}`, "{{z,y}}"},

		// The mirror rows: the note is on the body, so a produced brace
		// behind the group leaves the group alone, and a group beside the
		// abandoned one is read or not by where it stands.
		{"behind the group it changes nothing", `e={; echo {a,b}$e`, "a{ b{"},
		{"nor with text between", `e={; echo {a,b}x$e`, "ax{ bx{"},
		{"a group in front of the abandoned one still reads", `e={; echo {a,b}{c,d$e}`, "a{c,d{} b{c,d{}"},
		{"and the scan does not carry on past it", `e={; echo {c,d$e}{a,b}`, "{c,d{}{a,b}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := braceBody(t, tc.src, Yes)
			if st != 0 {
				t.Fatalf("status = %d, want 0; out = %q", st, got)
			}
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// An expansion that yields *fields of its own* puts the group's two braces in
// different words, so there is no group left for either reading to read.
//
// Written with the fields bracketed rather than echoed, which is the whole of
// whether the row says anything: `echo` joins its arguments with a blank, so
// the two fields `{1` and `2}` and the single field `{1 2}` print the same
// line. Measured 2026-09-27 on ksh93u+ and zsh 5.9.2, which agree — and an
// earlier form of this reading answered the single field.
//
// A group with a **written** comma beside the list — `x{p,$@}y` — is three
// words here and two in both of those shells, and that is a different
// question: the word is expanded once and the braces fan what came out, so a
// field boundary inside the group leaves no group. It is #4561 and it is not
// decided by how a body is read, so no row here asserts it.
func TestAListInABraceBodyLeavesNoGroup(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the body is the list", `set -- 1 2; printf '[%s]' {$@}`, "[{1][2}]"},
		{"a one-element list is still one field", `set -- 1; printf '[%s]' {$@}`, "[{1}]"},
		{"and an empty one leaves the braces", `set --; printf '[%s]' {$@}`, "[{}]"},

		// The control: the same shape with a scalar, where the body *is*
		// read after its expansion and the produced comma divides it.
		{"a scalar body is read", `e=1,2; printf '[%s]' {$e}`, "[1][2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := braceBody(t, tc.src, Yes)
			if st != 0 {
				t.Fatalf("status = %d, want 0; out = %q", st, got)
			}
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// The body's expansions run once, however many names the group makes.
//
// A word's values agree under both readings — the count is the whole of the
// tell, which is the same reason [Semantics.BraceFanExpandsEachNameOnItsOwn]
// is counted rather than read.
func TestABodyReadAfterExpansionRunsItsExpansionsOnce(t *testing.T) {
	const src = `i=0; echo {$((i=i+1)),z}; echo i=$i`
	got, st := braceBody(t, src, Yes)
	if st != 0 {
		t.Fatalf("status = %d, want 0; out = %q", st, got)
	}
	if want := "1 z\ni=1"; got != want {
		t.Errorf("%s = %q, want %q", src, got, want)
	}
}

// The count a redirection target takes is not this reading's, which is the
// same gate [Semantics.BraceRangeEndpointsExpanded] stands behind: the one
// column that reads a body after its expansions brace-expands an argument and
// not a target, so `e=x,y; : > {a,$e}` writes one file called `{a,x,y}`.
func TestARedirectionTargetDoesNotReadItsBodyAfterExpansion(t *testing.T) {
	const src = `e=x,y; : > {a,$e}; for f in *; do echo "$f"; done`
	got, st := braceBody(t, src, Yes)
	if st != 0 {
		t.Fatalf("status = %d, want 0; out = %q", st, got)
	}
	if want := "{a,x,y}"; got != want {
		t.Errorf("%s = %q, want %q", src, got, want)
	}
}
