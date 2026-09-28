// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// parenRun runs one snippet under the grammar that keeps a balanced `( … )`
// run standing behind a pattern group inside the word.
func parenRun(t *testing.T, src string, setup func(*Runner)) string {
	t.Helper()
	out, _ := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
		d.ExtendedPattern = true
		d.ExtendedPatternInCondition = true
		d.DoubleBracket = true
		d.ParamSubstitution = true
		d.CountedPatternGroup = true
		d.ParenRunAfterPatternGroupIsText = true
	}, setup)
	return strings.TrimSpace(out)
}

// A balanced `( … )` run standing immediately behind a pattern group stays in
// the word, and **what it then means is not one answer**: it is the characters
// in a word and in a condition, and a group in a `case` arm and in a
// parameter-expansion operand.
//
// That is the shell disagreeing with itself and it is reproduced rather than
// smoothed over. Measured 2026-09-28 against /bin/ksh `Version AJM 93u+
// 2012-08-01`; nothing here names a shell, because the reading is a grammar
// flag and a surface answer.
//
// **Every surface is a pair**, and the pairs are the whole point: one subject
// the text reading matches and one the group reading does. A row asserting
// only that `@(a)(b)` came back as the word it was written with would pass
// for *either* reading — in the reference because `(b)` is text and no file
// is called `a(b)`, and in a shell reading `(b)` as a group because no file
// is called `ab`. The fixtures below are what break that tie.
func TestAParenRunBehindAGroupReadsPerSurface(t *testing.T) {
	// Created here so that both readings have something to find. Without
	// `a(b)` on disk the glob rows agree for the wrong reason.
	dir := t.TempDir()
	for _, n := range []string{"ab", "a(b)", "a()", "a((b))", "a(b)(c)", "abc"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inDir := func(r *Runner) { r.Dir = dir }

	t.Run("a word reads the characters", func(t *testing.T) {
		for _, tc := range []struct{ name, src, want string }{
			{"a run behind a group", `printf "[%s]" @(a)(b)`, `[a(b)]`},
			{"an empty run", `printf "[%s]" @(a)()`, `[a()]`},
			{"a nested run", `printf "[%s]" @(a)((b))`, `[a((b))]`},
			{"a run behind a run", `printf "[%s]" @(a)(b)(c)`, `[a(b)(c)]`},
			// The control: two *quantified* groups are two groups, so this
			// names `ab` where every row above names the parenthesised
			// spelling. Without it the rows above would not say that the
			// parentheses reached the filesystem as characters.
			{"two groups name the joined text", `printf "[%s]" @(a)@(b)`, `[ab]`},
			// And the other control: quoting the run gives the same answer
			// the bare one does here, which is what says the bare one is
			// text rather than a group that happened to match.
			{"a quoted run names the same file", `printf "[%s]" @(a)"(b)"`, `[a(b)]`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := parenRun(t, tc.src, inDir); got != tc.want {
					t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
				}
			})
		}
	})

	t.Run("a condition reads the characters", func(t *testing.T) {
		for _, tc := range []struct{ name, src, want string }{
			{"the text matches", `[[ "a(b)" == @(a)(b) ]] && echo YES || echo NO`, "YES"},
			{"and the joined text does not", `[[ ab == @(a)(b) ]] && echo YES || echo NO`, "NO"},
			{"an alternation is text too", `[[ "a(b|c)" == @(a)(b|c) ]] && echo YES || echo NO`, "YES"},
			{"so neither arm matches", `[[ ab == @(a)(b|c) ]] && echo YES || echo NO`, "NO"},
			{"an empty run", `[[ "a()" == @(a)() ]] && echo YES || echo NO`, "YES"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := parenRun(t, tc.src, nil); got != tc.want {
					t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
				}
			})
		}
	})

	t.Run("a case arm reads a group", func(t *testing.T) {
		for _, tc := range []struct{ name, src, want string }{
			{
				"the joined text matches",
				`case ab in @(a)(b)) echo HIT;; *) echo MISS;; esac`, "HIT",
			},
			{
				"and the characters do not",
				`case "a(b)" in @(a)(b)) echo HIT;; *) echo MISS;; esac`, "MISS",
			},
			// The alternation rows are the discriminating pair: a run read
			// as text has no arms, so only a group can match `ac` and refuse
			// `ad`.
			{
				"an arm matches",
				`case ac in @(a)(b|c)) echo HIT;; *) echo MISS;; esac`, "HIT",
			},
			{
				"and a third letter does not",
				`case ad in @(a)(b|c)) echo HIT;; *) echo MISS;; esac`, "MISS",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := parenRun(t, tc.src, nil); got != tc.want {
					t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
				}
			})
		}
	})

	t.Run("an operand reads a group", func(t *testing.T) {
		for _, tc := range []struct{ name, src, want string }{
			{"the joined text is trimmed", `v=ab; printf "[%s]" "${v#@(a)(b)}"`, "[]"},
			{"and the characters are not", `v="a(b)"; printf "[%s]" "${v#@(a)(b)}"`, "[a(b)]"},
			// The control that says quoting still literalises where a group
			// is read, so the pair above is the *bare* run's doing.
			{"a quoted run is text there too", `v=ab; printf "[%s]" "${v#@(a)"(b)"}"`, "[ab]"},
			{"and trims the characters", `v="a(b)"; printf "[%s]" "${v#@(a)"(b)"}"`, "[]"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := parenRun(t, tc.src, nil); got != tc.want {
					t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
				}
			})
		}
	})
}

// The controls that bound the whole reading, and each is a way it could have
// been built too wide.
//
// A bare run with **no** group in front of it is a syntax error in a word and
// in a condition, so it is the group in front that licenses the run at all; a
// bare run *inside* a group is still a group, whichever surface it is on; and
// the run has to be adjacent to what the reading has already taken.
func TestAParenRunNeedsAGroupInFrontOfIt(t *testing.T) {
	t.Run("a bare run is still refused", func(t *testing.T) {
		for _, src := range []string{
			`[[ ab == a(b) ]]`,
			`case ab in a(b)) echo HIT;; esac`,
			`printf "[%s]" a(b)`,
		} {
			d := syntax.Core()
			d.ExtendedPattern = true
			d.ExtendedPatternInCondition = true
			d.DoubleBracket = true
			d.ParenRunAfterPatternGroupIsText = true
			if _, err := syntax.Parse(src+"\n", d); err == nil {
				t.Errorf("%q: read as a word, want the `(` to end it", src)
			}
		}
	})

	t.Run("a bare run inside a group is a group", func(t *testing.T) {
		// The arms keep their own reading, which matchGroup clears the flag
		// for: `@(a|(b))` matches `b`.
		for _, tc := range []struct{ src, want string }{
			{`[[ b == @(a|(b)) ]] && echo YES || echo NO`, "YES"},
			{`[[ q == @(a|(b)) ]] && echo YES || echo NO`, "NO"},
		} {
			if got := parenRun(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		}
	})

	t.Run("the dialect with bare groups keeps them", func(t *testing.T) {
		// **The reading must not reach the other dialect**, and this is not
		// a tidiness row: the matcher branch it arms is every dialect's,
		// where the count's identical line is reached only through the
		// counted-group scan. Where a bare `(` opens a group *anywhere*, a
		// second group behind the first is still a group.
		//
		// The row is a real prompt's, and arming it there turned the last
		// group into three ordinary characters — the line #1585 and #1217
		// are both about, found again by this change and kept here so it
		// cannot be found a third time.
		bare := func(t *testing.T, src string) string {
			t.Helper()
			out, _ := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
				d.PatternAlternation = true
				d.DoubleBracket = true
				d.ExtendedPattern = true
				d.ParenRunAfterPatternGroupIsText = true
			}, nil)
			return strings.TrimSpace(out)
		}
		for _, tc := range []struct{ name, src, want string }{
			{
				"two bare groups in one pattern",
				`[[ ab == (a)(b) ]] && echo YES || echo NO`, "YES",
			},
			{
				"and the second one still has arms",
				`[[ ac == (a)(b|c) ]] && echo YES || echo NO`, "YES",
			},
			{
				"while the characters do not match",
				`[[ "a(b)" == (a)(b) ]] && echo YES || echo NO`, "NO",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := bare(t, tc.src); got != tc.want {
					t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
				}
			})
		}
	})

	t.Run("the run has to be adjacent", func(t *testing.T) {
		d := syntax.Core()
		d.ExtendedPattern = true
		d.ExtendedPatternInCondition = true
		d.DoubleBracket = true
		d.ParenRunAfterPatternGroupIsText = true
		for _, tc := range []struct {
			src    string
			parses bool
		}{
			// One character between the group and the `(` ends the word, so
			// the rule is not "a word holding a group never ends at a `(`".
			{`printf "[%s]" @(a)x(b)`, false},
			{`printf "[%s]" @(a)(b)x(c)`, false},
			// And a `)` that some *other* construct produced licenses
			// nothing: the run is adjacent to a group, not to a byte.
			{`printf "[%s]" $(echo a)(b)`, false},
			{`printf "[%s]" "@(a)"(b)`, false},
			{`printf "[%s]" $((1))(b)`, false},
			{`printf "[%s]" \)(b)`, false},
			{`printf "[%s]" 'x)'(b)`, false},
			// Adjacency repeated is what makes a run behind a run chain, and
			// text behind the run is ordinary word text.
			{`printf "[%s]" @(a)(b)(c)`, true},
			{`printf "[%s]" @(a)(b)x`, true},
			// Every quantifier opens a group, so every one of them licenses
			// a run — including the counted spelling.
			{`printf "[%s]" ?(a)(b)`, true},
			{`printf "[%s]" *(a)(b)`, true},
			{`printf "[%s]" +(a)(b)`, true},
			{`printf "[%s]" !(a)(b)`, true},
		} {
			_, err := syntax.Parse(tc.src+"\n", d)
			if got := err == nil; got != tc.parses {
				t.Errorf("%q: parses = %v, want %v (err %v)", tc.src, got, tc.parses, err)
			}
		}
	})
}
