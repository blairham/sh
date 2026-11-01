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
		// Stars inside the literal as well (#6363).
		"a'*'c", "a'*'b'*'c", "ab'*'ab", "'*'a'*'c", "a'*'c'*'", "'*'b'*'c'*'", "é'*'é", "x'*'=y",
		// Neither shape, so the matcher answers either way.
		"a?c", "[ab]*", `a\*`, "*b?", "ab**", "a'**'c", "a'*'?c",
	}
	subjects := []string{
		"abc", "", "ab", "xabcx", "a b", "é", "-p", "ABC", "x=y", "a*", `\377\376`,
		"abab", "aXbYc", "acac", "éé", "x==y", "bcb",
	}
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

// TestEscapedPatternsAnswerAsTheMatcherDoes is the same comparison for the
// patterns a quoted operand turns into, where every metacharacter arrives
// with a backslash in front of it, and for values whose backslashes reach the
// matcher raw (#6423). A run with zsh's escape set is added, where a
// backslash before a character outside it stands for itself.
func TestEscapedPatternsAnswerAsTheMatcherDoes(t *testing.T) {
	// A leading `=` is a value: it is assigned to p and the operand is $p,
	// so its backslashes are the matcher's to read.
	patterns := []string{
		`"(x)"`, `"-p"`, `"a*"`, `"a*"'*'`, `'*'"[a]"`, `"a|b"'*'`, `"\\"`, `x"\\"'*'`,
		`"*"'*'`, `"a\\"'*'`, `"<1-2>"`, `"#x"`, `"^a"`, `"~a"`, `"!a"`, `"@(a)"`, `"x?"`,
		`"é("`, `"*"`, `'*'"*"`, `'*'"\\"`,
		`=bet\a`, `=a\`, `=a\\b`, `=a\*`, `=a\*'*'`, `=\é`, `=x\-y`, `=\(x\)`, `=a\\*`, `=a\b*`,
		`=\+`, `=x\{y`,
	}
	subjects := []string{
		"(x)", "-p", "a*", "a*b", "x[a]", "[a]", "a|bc", `\`, `x\y`, "**", "a\\b", "<1-2>",
		"#x", "^a", "~a", "!a", "@(a)", "x?", "é(", "beta", `bet\a`, "a", "ab", "x-y", `x\-y`,
		"+", `\+`, "x{y", `x\{y`, "",
	}
	runs := append(plainPatternRuns[:len(plainPatternRuns):len(plainPatternRuns)], plainPatternRun{
		name: "zsh's escapes", setup: func(r *Runner) {
			sem := permissive()
			sem.PatternEscapeReaches = "-=!*?[]()|^~#<>\\"
			r.Semantics = &sem
		},
	})
	for _, c := range runs {
		if c.extra != nil {
			// The extended grammar's own patterns are the other test's;
			// here it refuses the quoted groups for want of a dialect.
			continue
		}
		var b strings.Builder
		if c.records {
			b.WriteString("[[ SEED =~ SEED ]]\n")
		}
		for _, p := range patterns {
			pat := p
			if v, ok := strings.CutPrefix(p, "="); ok {
				head, tail, star := strings.Cut(v, "'*'")
				fmt.Fprintf(&b, "p='%s'\n", head)
				pat = "$p"
				if star {
					pat += "*" + tail
				}
			}
			for i, s := range subjects {
				fmt.Fprintf(&b, "s='%s'\n", s)
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
			gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
			for i := range min(len(gl), len(wl)) {
				if gl[i] != wl[i] {
					t.Errorf(
						"%s: the shortcut changed the answers for %s\nwith:    %q\nwithout: %q",
						c.name,
						patterns[min(i, len(patterns)-1)],
						gl[i],
						wl[i],
					)
				}
			}
			if len(gl) != len(wl) {
				t.Errorf("%s: %d lines with the shortcut, %d without", c.name, len(gl), len(wl))
			}
		}
		// The rows ran, and both readings were exercised: some escaped
		// pattern matched and some did not.
		if !strings.Contains(want, "c01 ") || !strings.Contains(want, "c00 ") {
			t.Errorf("%s: the rows did not run: %q", c.name, want)
		}
	}
}

// TestAngleLiteralsAnswerAsTheMatcherDoes is the same comparison for patterns
// holding `<` and `>`, which the shortcut reads as characters wherever they
// cannot open a numeric range, and for the stars between them (#6363). The
// patterns arrive through a variable so the grammar never reads the angle
// brackets as a redirection, and a run with numeric ranges on is the one
// where `<1-5>` and `<->` must stay the matcher's.
func TestAngleLiteralsAnswerAsTheMatcherDoes(t *testing.T) {
	patterns := []string{
		"<*>_default", "<base>_*", "*_<x>", "<x>", "<", ">", "a<b", "<-x", "<1-", "<1-*>", "<-*>",
		"<1-5>", "<->", "x<->y", "<*>_<1-2>", "<1-2>*", "*<9>*", "<**>", "<*>?",
	}
	subjects := []string{
		"<base>_default", "<theme>_palette", "<x>", "<", ">", "a<b", "3", "x12y", "<1-", "<1-5>",
		"<->", "<-x", "<1-7>", "<>_default", "_<x>", "<9>", "x<9>y", "<x>_<1-2>", "<x>_2", "",
	}
	runs := append(plainPatternRuns[:len(plainPatternRuns):len(plainPatternRuns)], plainPatternRun{
		name:   "numeric ranges",
		enable: func(g *syntax.Dialect) { g.NumericRangePattern = true },
		setup: func(r *Runner) {
			d := numRangeDialect()
			r.Dialect = &d
		},
	})
	for _, c := range runs {
		if c.extra != nil {
			continue
		}
		var b strings.Builder
		if c.records {
			b.WriteString("[[ SEED =~ SEED ]]\n")
		}
		for _, p := range patterns {
			fmt.Fprintf(&b, "p='%s'\n", p)
			for i, s := range subjects {
				fmt.Fprintf(&b, "s='%s'\n", s)
				fmt.Fprintf(&b, "if [[ $s == $p ]]; then printf 'c%d1 '; else printf 'c%d0 '; fi\n", i, i)
				if c.records {
					b.WriteString(`printf '<%s> ' "${M[*]}"` + "\n")
				}
				fmt.Fprintf(&b, "case $s in $p) printf 'k%d1 ';; *) printf 'k%d0 ';; esac\n", i, i)
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
			gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
			for i := range min(len(gl), len(wl)) {
				if gl[i] != wl[i] {
					t.Errorf("%s: the shortcut changed the answers for %s\nwith:    %q\nwithout: %q",
						c.name, patterns[min(i, len(patterns)-1)], gl[i], wl[i])
				}
			}
			if len(gl) != len(wl) {
				t.Errorf("%s: %d lines with the shortcut, %d without", c.name, len(gl), len(wl))
			}
		}
		// Both readings ran: the first pattern matches the first subject
		// and misses the second.
		if !strings.Contains(want, "c01 ") || !strings.Contains(want, "c10 ") {
			t.Errorf("%s: the rows did not run: %q", c.name, want)
		}
	}
	// And the range run did read ranges: `<1-5>` matched `3` there.
	d := numRangeDialect()
	out, _ := runGrammar(t, "p='<1-5>'; s=3; [[ $s == $p ]] && echo hit || echo miss\n",
		func(g *syntax.Dialect) { g.NumericRangePattern = true }, func(r *Runner) { r.Dialect = &d })
	if out != "hit\n" {
		t.Errorf("numeric ranges were not on in the range run: %q", out)
	}
}
