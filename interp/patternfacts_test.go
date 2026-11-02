// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The facts a substitution and a trim work out about a pattern before they
// look at a subject — the reach automaton, the written-arm variants, the
// capture plan, the span bounds — are kept per pattern and reading (#6380).
// These tests hold the kept facts to freshly read ones, and the written-arm
// search's shortcut to the walk it skips.

// armFactCases are the alternations where the written arm and the longest
// match part, so that a variant answered wrongly shows in the result.
var armFactCases = []struct {
	pattern  string
	subjects []string
}{
	{"(a|ab)", []string{"abc", "abab", "b"}},
	{"(ab|a)c", []string{"abc", "acabc"}},
	{"(a*|ab)", []string{"abc", "aab"}},
	{"(#b)(a|ab)(c|bcd)(d*)", []string{"abcd", "abcdd", "xabcdy"}},
	{"((#b)x|xy)*", []string{"xyz", "xxy"}},
	{"(x|y)(a|ab)", []string{"yab", "xa", "xab"}},
	{"([\\\\]|(%F))([\\{]([^\\}]##)[\\}])", []string{`\{a}x%F{b}`, "%F{bc}"}},
	{benchPattern, []string{benchSubject, "{a}{b}c", `\{x}`}},
}

// freshProgramCache empties the process's pattern cache for the test and
// again after it. The cache is shared by every test in the package, and a
// pattern these compile would otherwise reach a later test as a program where
// that test means to drive the walk.
func freshProgramCache(t *testing.T) {
	t.Helper()
	programs.m.Store(nil)
	t.Cleanup(func() { programs.m.Store(nil) })
}

// factOutcomes runs every substitution and longest trim the corpus has under
// o, the k-th reading, with the written-arm reading taken, and returns
// everything each one handed back. Under the race detector that is the
// reading's share of the corpus — see casesForReading.
//
// Every question is a piece of work of its own, run side by side with the
// rest (see eachInParallel), and the answers come back in the order the
// corpus lists them: by case, then subject, then question.
//
// A piece is one question and not one case, because one case is nearly the
// whole cost: under -race `(a|aa|aaa)##[a-z]` was 3.2s of a pass's 3.3s on
// one goroutine, its two failing subjects 1.6s each, while the other 254
// cases came to 0.1s between them (#7290). Pieces of one pattern also read
// its kept facts at the same time, which is the shape the shell has.
func factOutcomes(o patternOpts, k int) []string {
	arm := armOrder{answer: Yes, ask: func() bool { return true }}
	with := func(m matchReport, text string) string {
		return fmt.Sprintf("<%s|%v|%v>", text, m.groups, m.whole)
	}
	declined := func() EmptyMatchDeclinedPolicy { return EmptyMatchDeclinedAtTheEnd }
	cases := casesForReading(allProgramCases(), k)
	cases = append(cases, armFactCases...)
	// The questions, in the order their answers are listed: a substitution
	// under each anchor, once and then for all, and then each longest trim,
	// walked and then searched.
	anchors := []byte{0, '#', '%'}
	trims := []syntax.ParamOp{syntax.ParamTrimPrefixLong, syntax.ParamTrimSuffixLong}
	questions := 2*len(anchors) + 2*len(trims)
	type piece struct{ c, s, q int }
	var pieces []piece
	for c := range cases {
		for s := range cases[c].subjects {
			for q := range questions {
				pieces = append(pieces, piece{c, s, q})
			}
		}
	}
	out := make([]string, len(pieces))
	eachInParallel(len(pieces), func(n int) {
		p := pieces[n]
		pattern, subject := cases[p.c].pattern, cases[p.c].subjects[p.s]
		fresh := o
		fresh.where = &matchWhere{plan: planCapturesFor(pattern, o)}
		if p.q < 2*len(anchors) {
			e := replaceExprForTest(anchors[p.q/2], p.q%2 == 1)
			out[n] = replace(subject, pattern, e, fresh, arm, declined, with)
			return
		}
		q := p.q - 2*len(anchors)
		lo, hi, m, ok := trimSpan(subject, pattern, trims[q/2], fresh, arm, q%2 == 1)
		out[n] = fmt.Sprintf("%d %d %v %v %v", lo, hi, ok, m.groups, m.whole)
	})
	return out
}

// TestTheKeptFactsAnswerWhatAFreshReadingAnswers runs the corpus three ways
// under every reading: with nothing kept, with the facts kept and the cache
// cold, and again with it warm from every reading before it. The readings run
// one after another through the one cache, so a fact kept for one reading and
// handed to another is a difference here.
func TestTheKeptFactsAnswerWhatAFreshReadingAnswers(t *testing.T) {
	defer func(c bool) { patternFactsCached = c }(patternFactsCached)
	freshProgramCache(t)
	for k, rd := range programReadings() {
		patternFactsCached = false
		want := factOutcomes(rd.o, k)
		patternFactsCached = true
		cold := factOutcomes(rd.o, k)
		warm := factOutcomes(rd.o, k)
		for i := range want {
			if cold[i] != want[i] || warm[i] != want[i] {
				t.Errorf("%s: outcome %d is %q kept cold and %q kept warm, %q read afresh",
					rd.name, i, cold[i], warm[i], want[i])
			}
		}
	}
}

// TestTheArmShortcutAnswersWhatTheSearchAnswers holds onlyOneCanBegin to the
// walk it stands in for: the same corpus, every reading, with the shortcut on
// and off.
func TestTheArmShortcutAnswersWhatTheSearchAnswers(t *testing.T) {
	freshProgramCache(t)
	defer func(d bool) { armShortcutDisabled = d }(armShortcutDisabled)
	for k, rd := range programReadings() {
		armShortcutDisabled = true
		want := factOutcomes(rd.o, k)
		armShortcutDisabled = false
		got := factOutcomes(rd.o, k)
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: outcome %d is %q with the shortcut, %q without", rd.name, i, got[i], want[i])
			}
		}
	}
}

// TestTheThemeSubstitutionReadsItsPatternOnce is #6380's substitution counted
// in allocations rather than timed: a loop runs it with the pattern already
// read, so the second run must not read the pattern again. Before the facts
// were kept it built the reach automaton, the four written-arm variants with
// their plans and automata, and a unit-stop table per match: 449 allocations
// a run, options included, against 122 once they were kept.
func TestTheThemeSubstitutionReadsItsPatternOnce(t *testing.T) {
	freshProgramCache(t)
	e := &syntax.ParamExpr{All: true}
	arm := armOrder{answer: Yes, ask: func() bool { return true }}
	with := func(matchReport, string) string { return "X" }
	run := func() {
		o := benchOpts(t, benchPattern, benchSubject)
		if got := replace(benchSubject, benchPattern, e, o, arm, benchDeclined, with); got != "XXXXXXXXX" {
			t.Fatalf("the theme's substitution gave %q, want the nine X real zsh gives", got)
		}
	}
	run()
	if n := testing.AllocsPerRun(20, run); n > 250 {
		t.Errorf("the theme's substitution made %.0f allocations once its pattern had been read", n)
	}
}

// TestAKeptAutomatonForgetsALongerSubject is #6819: the reach analysis's
// scratch is kept on the pattern's automaton and handed to the next
// expansion, and it told one call's marks from the last call's by moving past
// this call's subject rather than the last one's. So a pattern expanded
// against a longer value and then a shorter one read the longer one's marks
// as its own, and `for x in xxxxxxxyz xyz; do echo "${x/x*z/Q}"; done` gave
// Q and then xyz. Each pair runs the longer subject first through one cache, and the
// shorter must answer as it does on its own.
func TestAKeptAutomatonForgetsALongerSubject(t *testing.T) {
	with := func(m matchReport, text string) string {
		return fmt.Sprintf("<%s|%v|%v>", text, m.groups, m.whole)
	}
	rd := programReadings()[0]
	// The program off, so that what answers is the walk the analysis
	// prunes, which is where the kept scratch is read.
	run := func(pattern, subject string) (got string) {
		withCompiling(false, func() {
			o := rd.o
			o.where = &matchWhere{plan: planCapturesFor(pattern, o)}
			got = replace(subject, pattern, replaceExprForTest(0, false), o, armOrder{},
				func() EmptyMatchDeclinedPolicy { return EmptyMatchDeclinedAtTheEnd }, with)
		})
		return got
	}
	for _, c := range []struct{ pattern, longer, shorter string }{
		{"(a|b)##c", "abababc", "abc"},
		{"x*z", "xxxxxxxyz", "xyz"},
	} {
		freshProgramCache(t)
		alone := run(c.pattern, c.shorter)
		freshProgramCache(t)
		run(c.pattern, c.longer)
		if after := run(c.pattern, c.shorter); after != alone {
			t.Errorf("%q on %q after %q: %q, and %q on its own", c.pattern, c.shorter, c.longer, after, alone)
		}
	}
}
