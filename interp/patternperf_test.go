// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
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
		got = replace(benchSubject, benchPattern, e, o, with)
	}
	if len(got) != 9 {
		b.Fatalf("result %q is %d bytes, want the 9 real zsh gives", got, len(got))
	}
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
