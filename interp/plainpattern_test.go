// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// plainPatternRun is one configuration the shortcut has to stay out of the
// way of: case folding on, a dialect that records what a pattern matched, and
// a grammar with extended patterns.
type plainPatternRun struct {
	name    string
	enable  func(*syntax.Dialect)
	setup   func(*Runner)
	records bool
	// extra are patterns only this configuration's grammar reads.
	extra []string
}

var plainPatternRuns = []plainPatternRun{
	{name: "core"},
	{name: "folding", setup: func(r *Runner) { r.SetMatchOption(MatchFoldsCase, true) }},
	{name: "recording", records: true, setup: func(r *Runner) {
		sem := permissive()
		sem.RegexMatchSurvivesAFailedMatch = Yes
		sem.RegexMatchOmitsGroupsThatDidNotMatch = Yes
		sem.PatternMatchWritesTheMatchRecord = Yes
		r.Semantics = &sem
		r.SetRegexMatch("M")
	}},
	{name: "extended", enable: quantifiedGroups, setup: func(r *Runner) {
		r.Dialect = quantifiedGroupDialect()
	}, extra: []string{"@(abc)", "+(a)bc", "!(x)", "?(a)bc", "@(ab|x)c"}},
}

// TestPlainPatternsAnswerAsTheMatcherDoes runs literal and star-ended
// patterns, and a few that are neither, through `[[ ]]` and `case` with the
// shortcut on and off, and requires the same output and status (#5873). The
// recording configuration also prints the record after every condition,
// which is what says a star pattern is left to the matcher there.
func TestPlainPatternsAnswerAsTheMatcherDoes(t *testing.T) {
	patterns := []string{
		"abc", "ab", "''", "'*'", "'**'", "ab'*'", "'*'bc", "'*'b'*'", "'*'é'*'",
		"-p", "x=y", "a.c", `"a b"`, "é", "AB'*'",
		// Neither shape, so the matcher answers either way.
		"a?c", "[ab]*", `a\*`, "*b?", "ab**",
	}
	subjects := []string{"abc", "", "ab", "xabcx", "a b", "é", "-p", "ABC", "x=y", "a*", `\377\376`}
	for _, c := range plainPatternRuns {
		var b strings.Builder
		if c.records {
			b.WriteString("[[ SEED =~ SEED ]]\n")
		}
		for _, p := range append(patterns[:len(patterns):len(patterns)], c.extra...) {
			// The stars are quoted above so the list reads as data; the
			// operand the matcher receives has them bare.
			pat := strings.ReplaceAll(p, "'*'", "*")
			for i, s := range subjects {
				if strings.HasPrefix(s, `\`) {
					fmt.Fprintf(&b, "s=$(printf '%s')\n", s)
				} else {
					fmt.Fprintf(&b, "s='%s'\n", s)
				}
				fmt.Fprintf(&b, "if [[ $s == %s ]]; then printf 'c%d1 '; else printf 'c%d0 '; fi\n", pat, i, i)
				if c.records {
					b.WriteString(`printf '<%s> ' "${M[*]}"` + "\n")
				}
				fmt.Fprintf(&b, "case $s in %s) printf 'k%d1 ';; *) printf 'k%d0 ';; esac\n", pat, i, i)
			}
			b.WriteString("printf '\\n'\n")
		}
		src := b.String()
		run := func(on bool) string {
			was := SetPlainPatternShortcutForTest(on)
			defer SetPlainPatternShortcutForTest(was)
			out, st := runGrammar(t, src, c.enable, c.setup)
			return fmt.Sprintf("%s|%d", out, st)
		}
		want, got := run(false), run(true)
		if got != want {
			t.Errorf("%s: the shortcut changed the answers\nwith:    %q\nwithout: %q", c.name, got, want)
		}
		if !strings.Contains(want, "c01 ") || !strings.Contains(want, "k01 ") {
			t.Errorf("%s: the rows did not run: %q", c.name, want)
		}
	}
}
