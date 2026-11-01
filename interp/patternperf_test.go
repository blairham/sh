// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// benchOpts is the options a dialect with alternation and the extended
// operators builds, by the constructs' names rather than by a shell's. The
// caret answer is the one this pattern needs and nothing else here does: a
// `^` inside a bracket is an extension, and an unanswered axis would make
// these benchmarks time a refusal.
func benchOpts(tb testing.TB, pattern, subject string) patternOpts {
	tb.Helper()
	d := syntax.Core()
	d.PatternAlternation = true
	sem := PosixSemantics()
	sem.BracketCaretNegates = Yes
	// testrunner:bare — the matcher is the subject and this Runner exists
	// only to answer patternOpts' questions. Nothing here opens a file, runs
	// a command or reaches a directory.
	r := &Runner{Semantics: &sem, Dialect: &d}
	r.SetMatchOption(ExtendedPatternOperators, true)
	return r.patternOpts(pattern, subject)
}

// The substitution #1383 was found on and #1398 is about: a prompt theme's
// pattern — nested alternation over closures with a bracket expression in
// every arm — applied to the 82-byte message it is written for.
//
// Real zsh 5.9.2 does this substitution in 12.4 µs on this machine, measured
// by running it 5000 times from a script and subtracting what the same script
// costs with no iterations at all.
const (
	benchPattern = `(#b)(([\\]|(%F))([\{]([^\}]##)[\}])|([\{]([^\}]##)[\}])([^\%\{\\]#))`
	benchSubject = `{error}Error{ehi}:{rst} Unknown subcommand{ehi}:{rst} ` +
		"{apo}`{cmd}lucid{apo}`{rst} "
)

// benchDeclined is the zsh reading of which empty match a replacement
// declines, which is the shell these benchmarks are measured against.
func benchDeclined() EmptyMatchDeclinedPolicy { return EmptyMatchDeclinedAtTheEnd }

// BenchmarkReplaceExtendedGlob times the matcher alone: one `${msg//pat/X}`
// with no shell around it, so a change inside the matcher is visible at the
// size it is rather than buried in process start and an rc file — which is
// what `make startup`'s rich-rc case, the other instrument for this, cannot
// avoid.
//
// The length is asserted because a matcher that stopped matching would run
// this loop very fast: nine bytes is the answer, and it is the answer real
// zsh gives for the same substitution.
func BenchmarkReplaceExtendedGlob(b *testing.B) {
	e := &syntax.ParamExpr{All: true}
	with := func(matchReport, string) string { return "X" }
	var got string
	for b.Loop() {
		o := benchOpts(b, benchPattern, benchSubject)
		got = replace(benchSubject, benchPattern, e, o, armOrder{}, benchDeclined, with)
	}
	if len(got) != 9 {
		b.Fatalf("result %q is %d bytes, want the 9 real zsh gives", got, len(got))
	}
	reportPerChar(b, benchSubject)
}

// reportPerChar adds what one substitution cost per character of its subject,
// which is the constant #6356 is about and the number that should move.
func reportPerChar(b *testing.B, subject string) {
	b.Helper()
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(len(subject)), "ns/char")
}

// BenchmarkReplaceExtendedGlobWithTheWrittenArmReading is the same
// substitution for the dialect that prefers the arm an alternation wrote
// first (#2152), which the theme's pattern has three of.
//
// It is a pair with the benchmark above and has to be read as one: the answer
// is the same nine bytes, so what the pair times is the *price of asking*.
// Measured on this machine, 560us against 655us — about 19% — and every bit
// of it is the second reading being consulted at each position a match was
// found at. Two things that look like the cost are not:
//
//   - the matcher questions. The arm search is bounded above by the length
//     reading's own edge, since a variant matches a subset of what the whole
//     pattern does, so the common case is *one* further question.
//   - the memo. The matcher drops it whenever the pattern moves, so asking
//     about a variant through the caller's matchWhere threw away everything
//     the whole-pattern scan had learned; armSearch carries one per variant
//     for that reason.
func BenchmarkReplaceExtendedGlobWithTheWrittenArmReading(b *testing.B) {
	e := &syntax.ParamExpr{All: true}
	with := func(matchReport, string) string { return "X" }
	arm := armOrder{answer: Yes, ask: func() bool { return true }}
	var got string
	for b.Loop() {
		o := benchOpts(b, benchPattern, benchSubject)
		got = replace(benchSubject, benchPattern, e, o, arm, benchDeclined, with)
	}
	if len(got) != 9 {
		b.Fatalf("result %q is %d bytes, want the 9 real zsh gives", got, len(got))
	}
	reportPerChar(b, benchSubject)
}

// BenchmarkOrdinaryGlobMatch is the other half of the measurement, and the
// one a change to the matcher can quietly ruin: `*.go` against a handful of
// filenames, which is what a shell spends nearly all of its matching life
// doing. Reading the pattern's structure once has to pay for itself here as
// well as on the theme's pattern — measured, it does, because the prescans it
// skips are paid at every position of even the shortest match.
func BenchmarkOrdinaryGlobMatch(b *testing.B) {
	o := benchOpts(b, "*.go", "somefile.go")
	names := []string{"somefile.go", "other.txt", "a.go", "README.md", "x", "deep_name_here.go"}
	matched := 0
	for b.Loop() {
		for _, s := range names {
			if matchPattern("*.go", s, o) {
				matched++
			}
		}
	}
	if matched == 0 {
		b.Fatal("nothing matched, so this timed a refusal rather than a match")
	}
}

// literalSubject is the sweep's subst_replace string repeated to 1584 bytes,
// long enough that the per-character constant swamps the fixed cost of a
// substitution.
var literalSubject = strings.Repeat("the_quick_brown_fox_jumps_over_the_lazy_dog_", 36)

// BenchmarkReplaceLiteralPerChar is `${s//o/0}` over a long subject, the
// shape #6356 measured at 4x bash, reported per character scanned.
func BenchmarkReplaceLiteralPerChar(b *testing.B) {
	e := &syntax.ParamExpr{All: true}
	with := func(matchReport, string) string { return "0" }
	var got string
	for b.Loop() {
		o := benchOpts(b, "o", literalSubject)
		got = replace(literalSubject, "o", e, o, armOrder{}, benchDeclined, with)
	}
	if want := strings.ReplaceAll(literalSubject, "o", "0"); got != want {
		b.Fatalf("result %q, want %q", got, want)
	}
	reportPerChar(b, literalSubject)
}

// The two substitutions above, counted in matcher questions rather than
// timed, so that losing what made them cheap fails a test instead of moving a
// number nobody is watching. matchWhere.asked is the matcher's own count of
// the steps it took, and it runs on for as long as the pattern and the
// subject stay the same — which is the whole of one substitution.
//
// Before #6356 the literal asked once at every unit of the subject, 1584
// questions for 144 replacements, and the theme's pattern asked 44,079: every
// span from the end of the message down, at each of the nine positions a
// match starts at.
func TestASubstitutionAsksTheMatcherOnlyWhereAMatchCouldBe(t *testing.T) {
	t.Parallel()
	e := &syntax.ParamExpr{All: true}
	o := benchOpts(t, "o", literalSubject)
	got := replace(literalSubject, "o", e, o, armOrder{}, benchDeclined,
		func(matchReport, string) string { return "0" })
	if want := strings.ReplaceAll(literalSubject, "o", "0"); got != want {
		t.Fatalf("the literal substitution gave %q, want %q", got, want)
	}
	if n, most := o.where.asked, strings.Count(literalSubject, "o"); n > most {
		t.Errorf("the literal substitution asked the matcher %d questions for %d occurrences", n, most)
	}

	o = benchOpts(t, benchPattern, benchSubject)
	got = replace(benchSubject, benchPattern, e, o, armOrder{}, benchDeclined,
		func(matchReport, string) string { return "X" })
	if got != "XXXXXXXXX" {
		t.Fatalf("the theme's substitution gave %q, want the nine X real zsh gives", got)
	}
	// About 2,200 on the change that set this, and a margin for the
	// matcher's own search to move inside without this failing.
	if n := o.where.asked; n > 5000 {
		t.Errorf("the theme's substitution asked the matcher %d questions for nine matches", n)
	}
}

// trimSubject is a line of words with the white space a trim is written for
// at both ends, and runs of it inside that the trim must leave alone.
var trimSubject = "  \t" + strings.Repeat("git  commit -m fix ", 100) + " \t "

// The trim F-Sy-H runs over every line it reads and every region it paints:
// `${x//((#s)[[:space:]]##|[[:space:]]##(#e))}`. Before #6423 the analysis
// that rules spans out refused both the anchors and the class, so the
// substitution asked the matcher about every span from every position of the
// value: 6,030,131 questions on this 1.9KB line, half a second where zsh
// 5.9.2 takes 0.3 ms.
func TestATrimSubstitutionAsksTheMatcherOnlyAtTheEnds(t *testing.T) {
	t.Parallel()
	const pattern = "((#s)[[:space:]]##|[[:space:]]##(#e))"
	e := &syntax.ParamExpr{All: true}
	o := benchOpts(t, pattern, trimSubject)
	got := replace(trimSubject, pattern, e, o, armOrder{}, benchDeclined,
		func(matchReport, string) string { return "" })
	if want := strings.TrimSpace(trimSubject); got != want {
		t.Fatalf("the trim gave %q, want %q", got, want)
	}
	// Only the two runs at the ends can match: 25 on the change that set
	// this, and a margin for the matcher's own steps to move inside.
	if n := o.where.asked; n > 200 {
		t.Errorf("the trim asked the matcher %d questions over %d bytes", n, len(trimSubject))
	}
}
