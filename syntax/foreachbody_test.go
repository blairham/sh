// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// What may stand where a `foreach` loop's body does (#3040).
//
// `end` had been taken for the whole of what the word adds, and a shipped
// completion function that writes `foreach c (…); do … done` is what says
// otherwise. Measured 2026-09-15 on zsh 5.9.2 — the only column in the panel
// that has the word at all, the other six calling the `(` a syntax error —
// each probe in a script file of its own and each printing both passes:
//
//	| probe                       | zsh 5.9.2 |
//	| `foreach c (a b); do … done`| `a` `b`   |
//	| `foreach c (a b) do … done` | `a` `b`   |
//	| `foreach c (a b) { … }`     | `a` `b`   |
//	| `foreach c (a b); … end`    | `a` `b`   |
//	| `foreach c (a b) … end`     | `a` `b`   |
//	| `foreach c in a b; do … done`| `a` `b`  |
//	| `foreach c in a b; … end`   | `a` `b`   |
//	| `foreach c (a b); do … end` | refused   |
//
// The last row is the half a wider reading would lose: the closers *pair*,
// so a `do` is closed by `done` and never by `end`, exactly as a `for` is
// never closed by `end`.
//
// The rows assert the loop's *body* rather than "it parsed", because parsing
// is the cheap half: a reading that took `do` as the first word of a body
// running to `end` would parse the first two rows and run a command called
// `do`.

func foreachBodyGrammar(d *Dialect) {
	d.Foreach = true
	// The parenthesized list and the brace body are flags of their own, and
	// rows below need both.
	d.ShortForm = true
	d.ForBraceBody = true
}

// foreachBody is the statement count of the first `foreach` in src, with the
// first statement's source text.
func foreachBody(t *testing.T, src string, d Dialect) (int, string) {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*ForClause)
	if !ok {
		t.Fatalf("%s: first command is %T, want a for clause", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	if len(c.Body) == 0 {
		return 0, ""
	}
	return len(c.Body), src[c.Body[0].Pos().Offset:c.Body[0].End().Offset]
}

func TestAForeachBodyIsNotOnlyTheOneEndCloses(t *testing.T) {
	t.Parallel()
	d := Core()
	foreachBodyGrammar(&d)
	for _, tc := range []struct{ name, src, want string }{
		{"a keyword body after a separator", "foreach c (a b); do echo one; done", "echo one"},
		{"a keyword body with no separator", "foreach c (a b) do echo one; done", "echo one"},
		{"a brace body with no separator", "foreach c (a b) { echo one; }", "echo one"},
		{"a brace body after a separator", "foreach c (a b); { echo one; }", "echo one"},
		{"the word's own closer", "foreach c (a b); echo one; end", "echo one"},
		{"the closer with no separator", "foreach c (a b) echo one; end", "echo one"},
		{"an in list and a keyword body", "foreach c in a b; do echo one; done", "echo one"},
		{"an in list and the word's own closer", "foreach c in a b; echo one; end", "echo one"},
		{"a keyword body over newlines", "foreach c (a b)\ndo\necho one\ndone", "echo one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n, first := foreachBody(t, tc.src, d)
			if n != 1 {
				t.Fatalf("%s: %d statements in the body, want 1", tc.src, n)
			}
			if first != tc.want {
				t.Errorf("%s: first statement %q, want %q", tc.src, first, tc.want)
			}
		})
	}
}

// TestAForeachClosersPair is the half that keeps the reading narrow: the
// shell this is measured from refuses a `do` closed by `end`, and refuses a
// `for` closed by `end` for the same reason.
func TestAForeachClosersPair(t *testing.T) {
	t.Parallel()
	d := Core()
	foreachBodyGrammar(&d)
	for _, src := range []string{
		"foreach c (a b); do echo one; end",
		"for c (a b); echo one; end",
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%s parsed, and the shell this is measured from refuses it", src)
		}
	}
}
