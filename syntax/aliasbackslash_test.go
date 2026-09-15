// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A backslash an alias body ends with (#2710).
//
// #2685 crossed the seam for constructs the body leaves *open* — a quote, a
// command substitution — by reading the body's unfinished tail joined to the
// input. A trailing backslash stopped being one of those in #2704: the lexer
// reads a backslash the input ends after as part of the word rather than as
// input that ran out, so the body's own lexer calls it finished and nothing
// carried. In the panel it carries anyway, because the panel is splicing text
// and the body's end is not the input's end.

// parsedWith is `parsed` under a dialect other than the core's.
func parsedWith(t *testing.T, d syntax.Dialect, a syntax.Aliases, src string) string {
	t.Helper()
	p := syntax.NewParser(src, d)
	p.Aliases = a
	f := p.Parse()
	if err := p.Err(); err != nil {
		return "error: " + err.Error()
	}
	return strings.TrimSpace(syntax.Print(f))
}

// The backslash reaches the character the input holds after the alias word,
// and escapes it. Six of the seven columns answer one word, and this is not
// an axis: it is the seam not being reached at all.
func TestABackslashAnAliasBodyEndsWithReachesTheInput(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			// The blank is escaped, so `a` and `b` are one word.
			"a blank after the alias word", `q b`, `printf "[%s]" a\ b`,
		},
		{
			// Any other character, for the same reason.
			"an ordinary character", `q "z"`, `printf "[%s]" a\ "z"`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := parsed(t, table("q", `printf "[%s]" a\`), c.src)
			if got != c.want {
				t.Errorf("%q expanded to %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// A body whose last token is finished takes none of the input, which is what
// keeps the widened test honest: reaching for the carry on every body that
// ends at a token boundary must not join a word to the one after it.
func TestAFinishedBodyStillTakesNoneOfTheInput(t *testing.T) {
	for _, c := range []struct {
		name, body, src, want string
	}{
		{"a plain word", "echo hi", "q there", "echo hi there"},
		{"an even run of backslashes", `echo a\\`, "q b", `echo a\\ b`},
		{"a closed quote", `echo "hi"`, "q there", `echo "hi" there`},
		{"an operator", "echo a;", "q b", "echo a; b"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := parsed(t, table("q", c.body), c.src)
			if got != c.want {
				t.Errorf("%q with body %q expanded to %q, want %q", c.src, c.body, got, c.want)
			}
		})
	}
}

// With nothing after the alias word the backslash meets the newline, where it
// is an ordinary line continuation — in five columns. The other two leave the
// line after it a line of its own. See Dialect.AliasBodyBackslashJoinsTheNextLine.
func TestWhetherThatBackslashJoinsTheNextLineFollowsTheAxis(t *testing.T) {
	const src = "q\necho two"
	joins := syntax.Core()
	joins.AliasBodyBackslashJoinsTheNextLine = true
	// Joined, the `echo` of the next line is part of the word rather than a
	// command of its own, and the line after it is what runs.
	if got, want := parsedWith(t, joins, table("q", `printf "[%s]" a\`), src),
		`printf "[%s]" aecho two`; got != want {
		t.Errorf("joining: got %q, want %q", got, want)
	}
	// Not joined, the next line is a line of its own — and the backslash is
	// still in the word, which is what the printer writes back escaped.
	want := `printf "[%s]" a` + "\\\\\necho two"
	if got := parsed(t, table("q", `printf "[%s]" a\`), src); got != want {
		t.Errorf("not joining: got %q, want %q", got, want)
	}
}

func TestTheAxisReachesOnlyTheNewline(t *testing.T) {
	joins := syntax.Core()
	joins.AliasBodyBackslashJoinsTheNextLine = true
	for _, d := range []syntax.Dialect{syntax.Core(), joins} {
		if got, want := parsedWith(t, d, table("q", `printf "[%s]" a\`), "q b"),
			`printf "[%s]" a\ b`; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
