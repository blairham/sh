// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// stepOverDialect is the core with the clause step-over on, and with empty
// compound bodies allowed because the two travel together: a dialect that
// refuses an empty body has already refused the clause by the time the
// step-over is asked, so the flag is inert without it. The one shell with
// the step-over has both.
func stepOverDialect() Dialect {
	d := Core()
	d.IfClauseStepsOverWhatItCannotUse = true
	d.EmptyCompoundBody = true
	return d
}

// A token no command could begin with, standing where an `if` or `elif`
// clause's first command was due, is stepped over and the one after it is
// refused (#3961).
//
// Measured 2026-09-21 on zsh 5.9.2 from a script file, `env -i
// PATH=/usr/bin:/bin LC_ALL=C zsh -f s.sh` with standard input on the null
// device. Written against the grammar flag rather than against the shell, as
// everything in this package is.
//
// It is not a message. The token is *consumed*, which is what makes
// `v=$(echo hi; if true; then)` a substitution that never closes there —
// TestAParenthesisTheGrammarSpentIsNoCloser's neighbor, and the row the
// dialect tests grade.
func TestAnIfClauseStepsOverWhatItCannotUse(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src string
		// want is the token the refusal names, or "" for the end of input.
		want string
	}{
		{"if true; then ) echo X; fi\n", "echo"},
		{"if true; then ) ; fi\n", "fi"},
		{"if true; then ) ; ; fi\n", "fi"},
		{"if true; then ) )\n", ")"},
		{"if true; then } echo X; fi\n", "echo"},
		{"if true; then done; fi\n", "fi"},
		{"if true; then do X; fi\n", "X"},
		{"if true; then :; elif true; then ) echo X; fi\n", "echo"},
		{"case x in a) if true; then ;; esac\n", "esac"},
		{"f() { if true; then } ; }\n", "}"},
		// **The clause need not be empty.** The question is asked wherever
		// the clause could still go on, not only where it never began, and
		// these three are what say so.
		{"if true; then :; ) echo X; fi\n", "echo"},
		{"case x in a) if true; then : ;; esac\n", "esac"},
		{"if true; then echo a; done; fi\n", "fi"},
		// The end of input is what comes next as readily as a token is, and
		// it is the row the substitution sweep turns on.
		{"if true; then )\n", ""},
	} {
		t.Run(c.src, func(t *testing.T) {
			_, err := Parse(c.src, stepOverDialect())
			se, ok := err.(*Error)
			if !ok {
				t.Fatalf("refusal = %v, want a syntax error", err)
			}
			if se.Token != c.want {
				t.Errorf("named %q, want %q", se.Token, c.want)
			}
		})
	}
}

// And what the clause must *not* step over, which is the whole of what makes
// the rule a rule rather than "a parenthesis is skipped here".
//
// Every row is “parse error near `)' “ — or the token it names — on zsh
// 5.9.2, measured the same day. `else`, every loop's `do`, a brace group, the
// and-or and pipeline operators one level out, a bare `!`, `time`, a `case`
// arm and an `if`'s own **condition** all name the token they met. The last
// is the sharpest: a refusal the condition raised is still standing when the
// clause is reached, and reading that as leave to step over the token the
// parser is on turns `if ) echo X; then :; fi` into `echo`.
func TestWhatAnIfClauseDoesNotStepOver(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{"if true; then :; else ) echo X; fi\n", ")"},
		{"while true; do ) echo X; done\n", ")"},
		{"until true; do ) echo X; done\n", ")"},
		{"for i in a; do ) echo X; done\n", ")"},
		{"{ ) echo X; }\n", ")"},
		{"true && ) echo X\n", ")"},
		{"true | ) echo X\n", ")"},
		{"! ) echo X\n", ")"},
		{"case x in ) echo X;; esac\n", ")"},
		{"case x in y) ) echo X;; esac\n", ")"},
		{"if ) echo X; then :; fi\n", ")"},
		{"if true; then :; elif ) echo X; then :; fi\n", ")"},
		// The operator that names itself in the one shell with this rule,
		// which is the same boundary it draws for a bare `!`.
		{"if true; then & echo X; fi\n", "&"},
	} {
		t.Run(c.src, func(t *testing.T) {
			_, err := Parse(c.src, stepOverDialect())
			se, ok := err.(*Error)
			if !ok {
				t.Fatalf("refusal = %v, want a syntax error", err)
			}
			if se.Token != c.want {
				t.Errorf("named %q, want %q", se.Token, c.want)
			}
		})
	}
	// And the clause's legal continuations, which are refused nowhere: there
	// is nothing to step over, and a rule that stepped over `fi` would eat
	// the `if`'s own closer. A `;` standing there is the same shape and is a
	// separate dialect's step-over rather than this one, so it is graded
	// where that flag is on.
	for _, src := range []string{
		"if true; then fi\n",
		"if true; then else echo X; fi\n",
		"if true; then :; elif true; then fi\n",
		"if true; then echo ok; fi\n",
	} {
		if _, err := Parse(src, stepOverDialect()); err != nil {
			t.Errorf("%q was refused: %v", src, err)
		}
	}
	// **The flag is the whole of it.** Without it every row above names the
	// token it met, which is what the other four presets do.
	plain := stepOverDialect()
	plain.IfClauseStepsOverWhatItCannotUse = false
	for _, src := range []string{"if true; then ) echo X; fi\n", "if true; then } echo X; fi\n"} {
		_, err := Parse(src, plain)
		se, ok := err.(*Error)
		if !ok {
			t.Fatalf("%q: refusal = %v, want a syntax error", src, err)
		}
		if se.Token == "echo" {
			t.Errorf("%q named %q with the flag off", src, se.Token)
		}
	}
}
