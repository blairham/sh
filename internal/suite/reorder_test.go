// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"strings"
	"testing"
)

// The canonicalisation is a pure function and is tested as one, which is the
// half of #3304 that does not need a suite to run.
//
// A scorer's failure mode is a discount that is slightly too wide: it would
// quietly write off a real disagreement and nothing downstream would ever say
// so. So every case below that asserts two runs become equal is paired with
// one that asserts they do not, and the pairs differ in one thing.

func TestPairsOnALineAreSortedAndNothingElseMoves(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{
			// The shape this exists for: a listed associative array, whose
			// sequence is the reference's hash order.
			"three pairs out of order",
			`declare -A a=([zebra]="3" [apple]="1" [mango]="2" )`,
			`declare -A a=([apple]="1" [mango]="2" [zebra]="3" )`,
		},
		{
			// One pair has no order to it. Rewriting it would only widen what
			// this function touches.
			"a single pair is left alone",
			`declare -A a=([zebra]="3" )`,
			`declare -A a=([zebra]="3" )`,
		},
		{
			// Everything outside the pairs stays byte for byte where it was,
			// including the text between two of them.
			"the surroundings are untouched",
			`before [b]="2" middle [a]="1" after`,
			`before [a]="1" middle [b]="2" after`,
		},
		{
			// A bracket inside a value must not end the pair early, or the
			// two halves sort as separate pairs and the line comes out
			// mangled rather than reordered.
			"a bracket inside a value",
			`([b]="x]y" [a]="1" )`,
			`([a]="1" [b]="x]y" )`,
		},
		{
			// The control that says this is the pair shape and not brackets:
			// a subscript with no value after it is not a pair.
			"a bare subscript is not a pair",
			`${a[2]} ${a[1]}`,
			`${a[2]} ${a[1]}`,
		},
		{
			"an ordinary line is untouched",
			`the quick brown fox`,
			`the quick brown fox`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sortedPairs(tc.in); got != tc.want {
				t.Errorf("sortedPairs(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestOneExpansionsWordsAreSortedAndTwoCallsAreNotMerged(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want []string
	}{
		{
			"one call, out of order",
			[]string{"argv[1] = <zebra>", "argv[2] = <apple>"},
			[]string{"argv[1] = <apple>", "argv[2] = <zebra>"},
		},
		{
			// The numbering bounds the run. Two calls in a row would
			// otherwise read as one, and sorting across the boundary would
			// move a word out of the expansion that produced it.
			"two calls stay two",
			[]string{"argv[1] = <b>", "argv[2] = <a>", "argv[1] = <d>", "argv[2] = <c>"},
			[]string{"argv[1] = <a>", "argv[2] = <b>", "argv[1] = <c>", "argv[2] = <d>"},
		},
		{
			// A single word has no order to it and is left where it is,
			// numbering included.
			"a one-word call is left alone",
			[]string{"argv[1] = <only>"},
			[]string{"argv[1] = <only>"},
		},
		{
			// A run that does not start at 1 is not a call this can see, so
			// nothing about it is rewritten.
			"a run that does not start at one",
			[]string{"argv[2] = <b>", "argv[3] = <a>"},
			[]string{"argv[2] = <b>", "argv[3] = <a>"},
		},
		{
			"lines that are not this shape at all",
			[]string{"one", "two", "three"},
			[]string{"one", "two", "three"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := canonicalOrder(tc.in)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("canonicalOrder(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

// Every row here is a pair: the same two runs with one thing changed, one of
// which must be steady and one of which must not. A discount that reported
// both as steady would be the failure this case exists to catch.
func TestTwoRunsAreSteadyOnlyWhenTheOrderIsTheWholeDifference(t *testing.T) {
	for _, tc := range []struct {
		name, a, b string
		want       bool
	}{
		{
			"the same keys in the reference's two orders",
			"declare -A a=([b]=\"2\" [a]=\"1\" )\n",
			"declare -A a=([a]=\"1\" [b]=\"2\" )\n",
			true,
		},
		{
			"a changed value is not an order",
			"declare -A a=([b]=\"2\" [a]=\"1\" )\n",
			"declare -A a=([a]=\"1\" [b]=\"9\" )\n",
			false,
		},
		{
			"a changed key is not an order",
			"declare -A a=([b]=\"2\" [a]=\"1\" )\n",
			"declare -A a=([a]=\"1\" [c]=\"2\" )\n",
			false,
		},
		{
			"one expansion's words in two orders",
			"argv[1] = <b>\nargv[2] = <a>\n",
			"argv[1] = <a>\nargv[2] = <b>\n",
			true,
		},
		{
			"a word that moved between two calls is not an order",
			"argv[1] = <b>\nargv[2] = <a>\nargv[1] = <d>\nargv[2] = <c>\n",
			"argv[1] = <a>\nargv[2] = <c>\nargv[1] = <b>\nargv[2] = <d>\n",
			false,
		},
		{
			"an extra line is not an order",
			"argv[1] = <b>\nargv[2] = <a>\n",
			"argv[1] = <a>\nargv[2] = <b>\nextra\n",
			false,
		},
		{
			"identical runs",
			"one\ntwo\n",
			"one\ntwo\n",
			true,
		},
		{
			// The whole point of the two shapes being the only shapes: two
			// plain lines that swapped places are a difference, not an order.
			"two ordinary lines that swapped is not an order",
			"one\ntwo\n",
			"two\none\n",
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameButForOrder(tc.a, tc.b); got != tc.want {
				t.Errorf("sameButForOrder(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// The figure is reported beside the differing lines and corrects nothing, so
// what it has to be right about is the two ends: an ordering difference is
// counted and a real one is not.
func TestTheReorderedFigureCountsOrderAndNotContent(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mine, theirs []string
		want         int
	}{
		{
			"a listing in two orders is entirely an order",
			[]string{`([a]="1" [b]="2" )`},
			[]string{`([b]="2" [a]="1" )`},
			1,
		},
		{
			"a changed value is not",
			[]string{`([a]="1" [b]="2" )`},
			[]string{`([b]="9" [a]="1" )`},
			0,
		},
		{
			"an ordinary disagreement is not",
			[]string{"one", "two"},
			[]string{"one", "three"},
			0,
		},
		{
			"runs that already agree have nothing to report",
			[]string{"one"},
			[]string{"one"},
			0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common, longest, _ := agreement(tc.mine, tc.theirs)
			got := reordered(tc.mine, tc.theirs, longest-common)
			if got != tc.want {
				t.Errorf("reordered(%q, %q) = %d, want %d", tc.mine, tc.theirs, got, tc.want)
			}
		})
	}
}

// The figure may never claim more than there were, whatever the
// canonicalisation does to the alignment.
func TestTheReorderedFigureStaysInsideTheDifferingLines(t *testing.T) {
	mine := []string{`([a]="1" [b]="2" )`, "one", "two"}
	theirs := []string{`([b]="2" [a]="1" )`, "three"}
	common, longest, _ := agreement(mine, theirs)
	differing := longest - common
	got := reordered(mine, theirs, differing)
	if got < 0 || got > differing {
		t.Errorf("reordered = %d, outside 0..%d", got, differing)
	}
}
