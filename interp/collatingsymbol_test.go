// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `[.x.]` is a collating element and `[=x=]` an equivalence class in four of
// the panel's columns, the ordinary characters they spell in one, and a
// sub-expression holding nothing in one more — see
// Semantics.CollatingElements. Which preset is which is asserted in the
// dialect packages and never here.

// collatingSem answers the axis over the permissive base, with the reading
// for a body the shell cannot take as one element named beside it: the two
// interact, and a test that fixed the second would be asserting a column
// nobody measured.
func collatingSem(a CollatingElementPolicy, unknown UnknownClassPolicy) Semantics {
	s := permissive()
	s.CollatingElements = a
	s.UnknownCharacterClass = unknown
	return s
}

// collates answers whether pattern matches subject with the axis at a.
func collates(t *testing.T, a CollatingElementPolicy, unknown UnknownClassPolicy, pattern, subject string) bool {
	t.Helper()
	src := `case "` + subject + `" in ` + pattern + `) echo Y;; *) echo n;; esac`
	out, st := run(t, src, withSem(collatingSem(a, unknown)))
	if st != 0 {
		t.Fatalf("%s vs %q: status %d, out %q", pattern, subject, st, out)
	}
	return strings.TrimSpace(out) == "Y"
}

// The construct, and the shape of the column that has not got it.
//
// A bracket is where the discrimination lives, and the `a]` row is why: with
// the construct off, `[[.a.]]` is the three-member set `[`, `.`, `a` and a
// literal `]` behind it, so it matches `a]` and not `a`. With it on it
// matches `a` and not `a]`. Every row below separates the two answers.
func TestACollatingElementIsOneMemberOfTheBracket(t *testing.T) {
	for _, tc := range []struct {
		pattern, subject string
		on, off          bool
	}{
		{"[[.a.]]", "a", true, false},
		{"[[.a.]]", "a]", false, true},
		{"[[=a=]]", "a", true, false},
		{"[[=a=]]", "a]", false, true},
		// Not only at the front of the bracket: a member may stand on
		// either side of one.
		{"[[.a.]x]", "x", true, false},
		{"[x[=a=]]", "x", true, false},
		// A range bound, at either end. `[[.a.]-c]` holds b where the
		// element is read and is the set `[`, `.`, `a`, `-`, `c` where it
		// is not.
		{"[[.a.]-c]", "b", true, false},
		{"[a-[.c.]]", "b", true, false},
		// With the construct off the `-` is still the range operator, over
		// an empty run, and the bracket ends at the element's own `]` — so
		// the same pattern matches `c]` there and `b` here. Measured
		// 2026-09-16 against both columns.
		{"[a-[.c.]]", "c]", false, true},
		// The delimiters are ordinary members with the construct off, which
		// is the other half of the same reading.
		{"[[.a.]]", "[]", false, true},
		{"[[.a.]]", ".]", false, true},
		// A `-` written as an element is a member and not the range
		// operator, so `[a[.-.]z]` holds three characters and not a run.
		{"[a[.-.]z]", "-", true, false},
		{"[a[.-.]z]", "b", false, false},
		// A `]` is reachable as an element, which is the one member an
		// ordinary bracket cannot spell anywhere but first.
		{"[[.].]]", "]", true, false},
	} {
		if got := collates(t, OneCharacterIsACollatingElement, UnknownClassIsInert,
			tc.pattern, tc.subject); got != tc.on {
			t.Errorf("on: %s vs %q = %v, want %v", tc.pattern, tc.subject, got, tc.on)
		}
		if got := collates(t, NoCollatingElements, UnknownClassIsInert,
			tc.pattern, tc.subject); got != tc.off {
			t.Errorf("off: %s vs %q = %v, want %v", tc.pattern, tc.subject, got, tc.off)
		}
	}
}

// A body of more than one character is not a collating element in the C
// locale, and what that does to the bracket around it is the same question a
// `[:name:]` the shell has not got asks. Measured 2026-09-16 with
// `[a[.nosuch.]b]`: the inert column matches a and b, the one that ends its
// scan matches a alone, the one that empties the bracket matches neither.
func TestABodyThatIsNotAnElementIsTheUnknownNameQuestion(t *testing.T) {
	for _, tc := range []struct {
		pattern, subject     string
		inert, ends, empties bool
	}{
		{"[a[.nosuch.]b]", "a", true, true, false},
		{"[a[.nosuch.]b]", "b", true, false, false},
		{"[a[=nosuch=]b]", "a", true, true, false},
		{"[a[=nosuch=]b]", "b", true, false, false},
		// Nothing of the body is a member under any of the three, which is
		// what separates "the element is inert" from "the delimiters are
		// ordinary characters".
		{"[a[.nosuch.]b]", "n", false, false, false},
		{"[a[.nosuch.]b]", ".", false, false, false},
		// The empty body is a body that is not an element either: `[[..]]`
		// holds no period.
		{"[a[..]b]", "a", true, true, false},
		{"[a[..]b]", ".", false, false, false},
	} {
		for _, c := range []struct {
			policy UnknownClassPolicy
			want   bool
		}{
			{UnknownClassIsInert, tc.inert},
			{UnknownClassEndsTheScan, tc.ends},
			{UnknownClassEmptiesTheBracket, tc.empties},
		} {
			if got := collates(t, OneCharacterIsACollatingElement, c.policy,
				tc.pattern, tc.subject); got != c.want {
				t.Errorf("%v: %s vs %q = %v, want %v",
					c.policy, tc.pattern, tc.subject, got, c.want)
			}
		}
	}
}

// A `[.` that nothing closes is not an element either, and the inert column's
// answer to that one is the characters standing for themselves rather than a
// skip — there is no element to step over. Measured 2026-09-16 with
// `[x[.a]y]`, whose closing `]` is the bracket's: bash matches `xy]`, `[y]`,
// `.y]` and `ay]`, dash matches `xy]` alone, ksh93 matches none of them.
func TestAnUnclosedCollatingDelimiterIsOrdinaryWhereTheReadingIsInert(t *testing.T) {
	for _, tc := range []struct {
		subject              string
		inert, ends, empties bool
	}{
		{"xy]", true, true, false},
		{"[y]", true, false, false},
		{".y]", true, false, false},
		{"ay]", true, false, false},
		{"qy]", false, false, false},
	} {
		for _, c := range []struct {
			policy UnknownClassPolicy
			want   bool
		}{
			{UnknownClassIsInert, tc.inert},
			{UnknownClassEndsTheScan, tc.ends},
			{UnknownClassEmptiesTheBracket, tc.empties},
		} {
			if got := collates(t, OneCharacterIsACollatingElement, c.policy,
				"[x[.a]y]", tc.subject); got != c.want {
				t.Errorf("%v: [x[.a]y] vs %q = %v, want %v",
					c.policy, tc.subject, got, c.want)
			}
		}
	}
}

// The delimiters are special inside a bracket expression and nowhere else, so
// a `[.` standing on its own in a pattern is an ordinary bracket and a period
// whichever way the axis reads.
func TestTheDelimitersAreSpecialOnlyInsideABracket(t *testing.T) {
	for _, a := range []CollatingElementPolicy{
		OneCharacterIsACollatingElement, NoCollatingElements,
		CollatingElementsHoldNothing, ACollatingElementMayBeNamed,
	} {
		for _, tc := range []struct {
			pattern, subject string
			want             bool
		}{
			{"a[.b]", "a.", true},
			{"a[.b]", "ab", true},
			{"a[.b]", "a[", false},
			{"x[=y]", "x=", true},
		} {
			if got := collates(t, a, UnknownClassIsInert, tc.pattern, tc.subject); got != tc.want {
				t.Errorf("%v: %s vs %q = %v, want %v", a, tc.pattern, tc.subject, got, tc.want)
			}
		}
	}
}

// The axis is asked only where a pattern really opens one, the rule the
// caret, bracket and unknown-name axes follow — so the core, which answers
// nothing, still matches an ordinary bracket and refuses only the pattern
// that poses the question.
func TestTheCollatingAxisIsAskedOnlyWhenItApplies(t *testing.T) {
	if out, st := run(t, `case b in [abc]) echo in;; *) echo out;; esac`,
		withSem(CoreSemantics())); out != "in\n" || st != 0 {
		t.Errorf("an ordinary bracket in the core: got %q status %d", out, st)
	}
	if out, st := run(t, `case . in a[.b]) echo in;; *) echo out;; esac`,
		withSem(CoreSemantics())); out != "out\n" || st != 0 {
		t.Errorf("a `[.` outside a bracket in the core: got %q status %d", out, st)
	}
	if _, st := run(t, `case a in [[.a.]]) echo in;; *) echo out;; esac`,
		withSem(CoreSemantics())); st != 2 {
		t.Errorf("the core should refuse a collating element, status %d", st)
	}
}

// TestTheDelimitersMayBeReadWithNoBodyEverAnElement is the third reading, and
// it is the one a boolean could not hold: the delimiters are a sub-expression
// — so the `]` inside them does not end the bracket — and no body is ever an
// element, so the set they contribute to is empty.
//
// Measured 2026-09-18 in BusyBox v1.37.0 under `LC_ALL=C`: `[[.a.]]` matches
// nothing at all there, where the columns that read an element match `a` and
// the column with no construct matches `a]`. The second row is what says the
// delimiters were read rather than the bracket having simply failed — a
// member written behind the element still counts.
func TestTheDelimitersMayBeReadWithNoBodyEverAnElement(t *testing.T) {
	for _, tc := range []struct {
		pattern, subject            string
		nothing, oneChar, noneAtAll bool
	}{
		// The discriminating row: three readings, three answers.
		{"[[.a.]]", "a", false, true, false},
		{"[[.a.]]", "a]", false, false, true},
		{"[[.a.]]", "[", false, false, false},
		// The delimiters were read, which an empty set alone cannot say.
		{"[[.a.]x]", "x", true, true, false},
		{"[[.a.]x]", "a", false, true, false},
		{"[x[=a=]]", "x", true, true, false},
		// A one-character body is no different from any other body here, so
		// the unknown-body reading answers every one of them: with it inert,
		// the members beside the element still count.
		{"[a[.b.]c]", "a", true, true, false},
		{"[a[.b.]c]", "c", true, true, false},
		{"[a[.b.]c]", "b", false, true, false},
	} {
		for _, c := range []struct {
			policy CollatingElementPolicy
			want   bool
		}{
			{CollatingElementsHoldNothing, tc.nothing},
			{OneCharacterIsACollatingElement, tc.oneChar},
			{NoCollatingElements, tc.noneAtAll},
		} {
			got := collates(t, c.policy, UnknownClassIsInert, tc.pattern, tc.subject)
			if got != c.want {
				t.Errorf("%v: %s vs %q = %v, want %v",
					c.policy, tc.pattern, tc.subject, got, c.want)
			}
		}
	}
}

// TestACollatingElementMayBeWrittenAsAName is the fourth reading. A body of
// more than one character is not an element in the C locale, and one column
// looks it up as a **name** from the portable character set instead.
//
// Measured 2026-09-18 on bash 5.3.20 under `LC_ALL=C`, which is the only
// column in the panel that does it: ksh93u+ and dash 0.5.12 read the same
// bodies as bodies that are not elements, so `[[.hyphen.]]` matches no `-`
// there. The roster and the sweep behind it are in interp/collatingname.go.
func TestACollatingElementMayBeWrittenAsAName(t *testing.T) {
	for _, tc := range []struct {
		pattern, subject string
		named, oneChar   bool
	}{
		// A name under either delimiter.
		{"[[.hyphen.]]", "-", true, false},
		{"[[=hyphen=]]", "-", true, false},
		{"[[.period.]]", ".", true, false},
		{"[[=space=]]", " ", true, false},
		// Nothing of the name's own text is ever a member, under either
		// reading: where it is a name it stands for one character, and
		// where it is not it is a body that is not an element.
		{"[[.hyphen.]]", "h", false, false},
		{"[[.hyphen.]]", "n", false, false},
		// A name is a member of a wider set and a bound of a range, which
		// is where the lookup has to return the character rather than a
		// verdict.
		{"[[.hyphen.]q]", "-", true, false},
		{"[[.hyphen.]q]", "q", true, true},
		{"[[.zero.]-[.nine.]]", "5", true, false},
		{"[[.zero.]-[.nine.]]", "a", false, false},
		// A body of one character is the element it spells under both
		// readings, so the roster is asked about names and never about the
		// C locale.
		{"[[.a.]]", "a", true, true},
		// And a body the roster has not got is a body that is not an
		// element, exactly as a longer one is where no names are read. The
		// character `_` is reached as `underscore` in the column that has
		// the roster, which is what makes this row a measurement rather
		// than a guess.
		{"[[.low-line.]]", "_", false, false},
		{"[[.underscore.]]", "_", true, false},
		{"[[.nosuch.]]", "n", false, false},
	} {
		for _, c := range []struct {
			policy CollatingElementPolicy
			want   bool
		}{
			{ACollatingElementMayBeNamed, tc.named},
			{OneCharacterIsACollatingElement, tc.oneChar},
		} {
			got := collates(t, c.policy, UnknownClassIsInert, tc.pattern, tc.subject)
			if got != c.want {
				t.Errorf("%v: %s vs %q = %v, want %v",
					c.policy, tc.pattern, tc.subject, got, c.want)
			}
		}
	}
}
