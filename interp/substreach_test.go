// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
	"testing"
)

// endSetCases are the shapes the reach analysis reads — closures, groups,
// quantified groups, brackets beside them — with subjects chosen so that
// matches of several lengths start at the same position.
var endSetCases = []struct{ pattern, subject string }{
	{benchPattern, benchSubject},
	{benchPattern, `\{a}x%F{b}yy{c}\z{}{d`},
	{"(#b)(a|ab)*", "abcab"},
	{"(#m)a*", "xaayaaz"},
	{"x#(*)", "xx93x"},
	{"(a|ab)c", "abcacabc"},
	{"(ab|a)#c", "ababacabc"},
	{"[^}]##", "ab}c}}d"},
	{"{[^}]##}", "{a}{}{bc}x{"},
	{"{[^}]##}[^%{]#", "{a}xy{b}%z{"},
	{"+(a)b", "aab ab b"},
	{"@(a|b)c", "acbccc"},
	{"*(ab)", "ababxab"},
	{"?(a)b", "abbab"},
	{"!(a)b", "ab xb"},
	{"a*b", "aabxbab"},
	{"*a", "bab"},
	{"**(e|f)", "abef"},
	{"(|a)b", "abb"},
	{"(a#)#", "aaba"},
	{"((a|b)#c)##", "abcacbcx"},
	{"[ab]#c", "abcbcc"},
	{`\*#x`, "**x*x"},
	{`a\`, `a\b`},
	{"(a|b", "a(a|b"},
	{"a(#s)b", "ab"},
	{"(#s)a#", "aab"},
	{"a#(#e)", "baa"},
	{"((#s)[[:space:]]##|[[:space:]]##(#e))", "  a  b \tc  "},
	{"((#s) ##| ##(#e))", "  a  b  c  "},
	{"((#s)x|y(#e))#", "xyxy"},
	{"(#b)*$\\(([^\\)]#)(\\)|(#e))", "a $(b) c $(d"},
	{"(a|(#s))b", "abbab"},
	{"*(#s)ab", "abab"},
	{"(#e)", "ab"},
	{"(#s)", "ab"},
	{"(#i)a#", "aAa"},
	{"a~b*", "ab ac"},
	{"^a#", "aab"},
	{"<1-3>#", "123x"},
	{"(#c2)a", "aaa"},
	{"a(#c1,2)", "aaa"},
	{"[[:alpha:]]#", "ab1"},
	{"[^[:space:][:digit:]]##", "ab 12c d"},
	{"[[:upper:]x]#y", "AxBy y"},
	{"[[.a.]]#", "aab"},
	{"[[=a=]b]#", "aab"},
	{"[[:alp]x]", "[:x]"},
	{"[[:bogus:]]a", "aa"},
	{"é#", "ééa"},
	{"?é", "aéé"},
	{"[éa]#b", "éab"},
	{"(é|a)##", "éaéb"},
	{"*", "abc"},
	{"", "abc"},
	{"x#", ""},
}

// reachReadings are the option sets the analysis has to agree with the
// matcher under: the widest reading over both units, a reading with
// quantified groups and no bare ones, and one splitting a live `|` at the
// top.
func reachReadings() []struct {
	name string
	o    patternOpts
} {
	out := spanReadings()
	quant := patternOpts{quantified: true}
	quantChars := quant
	quantChars.chars = true
	top := spanOptsForTest()
	top.topGroup = true
	return append(out, []struct {
		name string
		o    patternOpts
	}{{"quantified", quant}, {"quantified characters", quantChars}, {"top-level bar", top}}...)
}

// **The set may only ever leave out an end the matcher would refuse.**
//
// That is the whole of the analysis' safety, asserted directly: for every
// start and every end of every subject, a span the matcher accepts must be
// one the set holds. A set that holds too much costs attempts; one that
// holds too little silently drops a replacement, and nothing else in the
// suite would say which span went missing.
//
// It is also required to hold *exactly* the accepted ends wherever it
// answers, which is stronger than safety and is what says the analysis read
// the pattern the way the matcher does rather than more loosely: a set that
// agreed only by being wide would still be safe and would buy nothing.
func TestTheReachSetHoldsEveryEndTheMatcherAccepts(t *testing.T) {
	t.Parallel()
	read := 0
	for _, rd := range reachReadings() {
		for _, tc := range append(append(endSetCases, spanCases...), edgeCases...) {
			o := rd.o
			o.where = &matchWhere{}
			r, ok := newReachSet(tc.pattern, o)
			if !ok {
				continue
			}
			read++
			stops := unitStops(tc.subject, o)
			stop := make([]bool, len(tc.subject)+1)
			for _, x := range stops {
				stop[x] = true
			}
			e := newReachEnds(r, tc.subject, stops)
			for a, i := range stops {
				any := e.from(i)
				for _, j := range stops[a:] {
					o.where = &matchWhere{}
					accepted, _ := matchPatternIn(tc.pattern, tc.subject[i:j], tc.subject, i, o)
					switch {
					case !e.sure:
						continue
					case accepted && (!any || !e.may(j)):
						t.Errorf("%s: pattern %q on %q: %d..%d matches and the reach set leaves it out",
							rd.name, tc.pattern, tc.subject, i, j)
					case !accepted && e.may(j):
						t.Errorf("%s: pattern %q on %q: %d..%d does not match and the reach set holds it",
							rd.name, tc.pattern, tc.subject, i, j)
					}
				}
				for _, x := range e.marked {
					if !stop[x] {
						t.Errorf("%s: pattern %q on %q: the reach set holds %d, which is not a unit boundary",
							rd.name, tc.pattern, tc.subject, x)
					}
				}
			}
		}
	}
	// The other half of a null: an analysis that refused everything would
	// pass every row above.
	if read < 150 {
		t.Errorf("the analysis read only %d of the pattern-reading pairs, so the rows above tested little", read)
	}
}

// The patterns it exists for must be ones it reads, and the constructs it
// does not reason about must be ones it refuses rather than guesses at.
func TestTheReachSetReadsWhatItClaimsAndRefusesTheRest(t *testing.T) {
	t.Parallel()
	o := spanOptsForTest()
	for _, p := range []string{
		benchPattern, "a*b", "(a|ab)c", "[^}]##", "+(a)b", "?(a)b", "*(ab)", "(#b)(a|ab)*", "x#",
		"(#s)a", "a(#e)", "((#s) ##| ##(#e))", "[[:alpha:]]", "((#s)[[:space:]]##|[[:space:]]##(#e))",
	} {
		if _, ok := newReachSet(p, o); !ok {
			t.Errorf("pattern %q is refused, and it is one the analysis is for", p)
		}
	}
	for _, p := range []string{
		"!(a)b", "a~b", "^a", "<1-3>", "(#i)a", "(#c2)a", "a(#c1,2)", `a\`, "[ab",
		"[[.a.]]", "[[=a=]]", "[[:alp]", "[a[b]",
	} {
		if _, ok := newReachSet(p, o); ok {
			t.Errorf("pattern %q is read, and it is a construct the analysis has no reading of", p)
		}
	}
	folded := o
	folded.fold = true
	if _, ok := newReachSet("a*b", folded); ok {
		t.Error("a folding comparison is read, and every byte the analysis compares would be wrong")
	}
}

// And the substitution built on it must answer what it answered without it,
// down to what each replacement was handed: the text, and every `(#b)` span.
func TestSubstitutionAnswersTheSameWithTheReachAsWithout(t *testing.T) {
	with := func(m matchReport, text string) string {
		return fmt.Sprintf("<%s|%v|%v>", text, m.groups, m.whole)
	}
	run := func(o patternOpts, pattern, subject string, anchor byte, all, reach bool) string {
		defer func(prev bool) { reachDisabled = prev }(reachDisabled)
		reachDisabled = !reach
		o.where = &matchWhere{}
		return replace(subject, pattern, replaceExprForTest(anchor, all), o, armOrder{},
			func() EmptyMatchDeclinedPolicy { return EmptyMatchDeclinedAtTheEnd }, with)
	}
	for _, rd := range reachReadings() {
		for _, tc := range append(append(endSetCases, spanCases...), edgeCases...) {
			for _, anchor := range []byte{0, '#', '%'} {
				for _, all := range []bool{false, true} {
					got := run(rd.o, tc.pattern, tc.subject, anchor, all, true)
					want := run(rd.o, tc.pattern, tc.subject, anchor, all, false)
					if got != want {
						t.Errorf("%s: pattern %q on %q anchor %q all=%v: with the reach %q, without %q",
							rd.name, tc.pattern, tc.subject, anchor, all, got, want)
					}
				}
			}
		}
	}
	// The same through options a Runner built, which is where `(#b)` plans
	// its groups and so where a report has spans in it to compare.
	for _, tc := range endSetCases {
		if strings.Contains(tc.pattern, "é") {
			continue
		}
		o := benchOpts(t, tc.pattern, tc.subject)
		got := run(o, tc.pattern, tc.subject, 0, true, true)
		want := run(o, tc.pattern, tc.subject, 0, true, false)
		if got != want {
			t.Errorf("pattern %q on %q through a Runner: with the reach %q, without %q",
				tc.pattern, tc.subject, got, want)
		}
	}
}
