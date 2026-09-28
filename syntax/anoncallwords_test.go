// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// bareKeywordLeadingRedirs is bareKeyword with the rule that lets a
// redirection stand in front of a compound command.
//
// The `()` header needs it and the keyword header does not, which is a fact
// about where the two positions live rather than an inconsistency: `function
// >f body` is a position of the *function* grammar — the named spelling
// `function g >f { … }` has it too — while `() 2>e { … }` is the ordinary
// leading redirection, read by the reader that puts one in front of any
// compound. One dialect for every row below, so that what the rows compare is
// where the redirection landed and not which grammar each was allowed by.
func bareKeywordLeadingRedirs() Dialect {
	d := bareKeyword()
	d.RedirectionBeforeACompound = RedirectionMayPrecedeAnyCompoundCommand
	return d
}

// anonOf digs the nameless function out of a one-statement file.
func anonOf(t *testing.T, src string, d Dialect) *AnonFunc {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("%q came to %d statements, want 1", src, len(f.Stmts))
	}
	fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*AnonFunc)
	if !ok {
		t.Fatalf("%q came to %T, want an AnonFunc", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	return fn
}

// Words and redirections behind a nameless function's body, and the two
// headers do not read them the same way (#5079).
//
// Written against the grammar flag rather than a shell, as everything in this
// package is; the behavior is graded in dialect/zsh. What these rows assert
// is which node each redirection landed on, because that is where the fault
// was: a word after a redirection was refused under the `()` header and taken
// under the keyword's, and both are the wrong way round.
func TestTheHeaderDecidesWhetherWordsAndRedirectionsInterleave(t *testing.T) {
	const body = "{ :; }"
	for _, tc := range []struct {
		name, src    string
		args, redirs int
		refused      bool
	}{
		// The parenthesised header reads its words the way a simple command
		// reads its own.
		{name: "() with one between the words", src: "() " + body + " a 2>e b\n", args: 2, redirs: 1},
		{name: "() with two", src: "() " + body + " a 2>e b 3>g c\n", args: 3, redirs: 2},
		{name: "() with them all behind", src: "() " + body + " a b 2>e\n", args: 2, redirs: 1},
		{name: "() with them all in front", src: "() " + body + " 2>e a b\n", args: 2, redirs: 1},
		// The keyword header takes words, then redirections, and a word
		// after one is a refusal.
		{name: "the keyword with words alone", src: "function " + body + " a b\n", args: 2},
		{name: "the keyword with one behind them", src: "function " + body + " a b >f\n", args: 2, redirs: 1},
		{name: "the keyword with one among them", src: "function " + body + " a >f b\n", refused: true},
		{name: "the keyword with one in front of them", src: "function " + body + " >f a b\n", refused: true},
		// The body on a later line is the same header and answers the same,
		// which is the row that says the rule is keyed on the header word
		// and not on how the body was reached.
		{name: "the keyword over a line, one among them", src: "function\n" + body + " a >f b\n", refused: true},
		{name: "the keyword over a line, words alone", src: "function\n" + body + " a b\n", args: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.refused {
				if _, err := Parse(tc.src, bareKeyword()); err == nil {
					t.Errorf("%q parsed, want a refusal", tc.src)
				}
				return
			}
			fn := anonOf(t, tc.src, bareKeyword())
			if len(fn.Args) != tc.args {
				t.Errorf("%d arguments, want %d", len(fn.Args), tc.args)
			}
			if len(fn.Redirs) != tc.redirs {
				t.Errorf("%d redirections on the call, want %d", len(fn.Redirs), tc.redirs)
			}
		})
	}
}

// A redirection written in front of the body is the **body's**, and one
// written behind it is the **call's**.
//
// Both reach the body node through the same list — a leading redirection is
// prepended to the compound it stands in front of, which is the body group —
// so they are told apart by where they were written. Counting them on the two
// nodes is what grades that split; the behavior it produces is a trace line
// that lands in the file or does not, which dialect/zsh measures.
func TestARedirectionInFrontOfTheBodyStaysOnTheBody(t *testing.T) {
	for _, tc := range []struct {
		name, src      string
		onCall, onBody int
	}{
		{"in front", "() 2>e { :; }\n", 0, 1},
		{"behind", "() { :; } 2>e\n", 1, 0},
		{"one of each", "() 2>e { :; } 3>g\n", 1, 1},
		{"one of each, with words between", "() 2>e { :; } a 3>g b\n", 1, 1},
		{"the keyword header, in front", "function 2>e { :; }\n", 0, 1},
		{"the keyword header, behind", "function { :; } 2>e\n", 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := anonOf(t, tc.src, bareKeywordLeadingRedirs())
			if len(fn.Redirs) != tc.onCall {
				t.Errorf("%d redirections on the call, want %d", len(fn.Redirs), tc.onCall)
			}
			g, ok := fn.Body.(*Group)
			if !ok {
				t.Fatalf("body came to %T, want a group", fn.Body)
			}
			if len(g.Redirs) != tc.onBody {
				t.Errorf("%d redirections on the body, want %d", len(g.Redirs), tc.onBody)
			}
		})
	}
}

// The keyword followed by a redirection takes the command behind it as a
// body, and the redirection goes in front of that body.
//
// A word there is a *body* and not a name, which is the fact the whole branch
// turns on and the one that cannot be read off the spelling: `function foo`
// makes `foo` a name and `function >f foo` runs it.
func TestTheKeywordTakesABodyBehindARedirection(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		bodyIs    string
		leading   int
	}{
		{"a brace body", "function >f { :; }\n", "*syntax.Group", 1},
		{"a simple command", "function >f :\n", "*syntax.SimpleCmd", 1},
		{"a subshell", "function >f ( : )\n", "*syntax.Subshell", 1},
		{"two redirections in front", "function >f 2>e { :; }\n", "*syntax.Group", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := anonOf(t, tc.src, bareKeyword())
			if fn.Bare {
				t.Error("Bare is true, so the command behind the redirection was not read as a body")
			}
			if got := anonBodyTypeName(fn.Body); got != tc.bodyIs {
				t.Errorf("body came to %s, want %s", got, tc.bodyIs)
			}
			if n := len(fn.Redirs); n != 0 {
				t.Errorf("%d redirections on the call, want 0: one in front of the body is the body's", n)
			}
			if n := anonBodyRedirCount(fn.Body); n != tc.leading {
				t.Errorf("%d redirections on the body, want %d", n, tc.leading)
			}
		})
	}
	// The other bound: a word that **closes the construct around it** is
	// not a body either, and the keyword stands alone in front of it. These
	// four are the whole of what the stop-word half of that test does —
	// without it each one is read as a body and the parse fails on the word
	// that was going to end the construct.
	for _, src := range []string{
		"{ function >f }\n",
		"for i in 1; do function >f done\n",
		"if true; then function >f fi\n",
		"case x in x) function >f esac\n",
	} {
		if _, err := Parse(src, bareKeyword()); err != nil {
			t.Errorf("%q was refused (%v), want the keyword standing alone in front of the word", src, err)
		}
	}
	// And the bound. A separator between the redirection and the body is a
	// token of its own, so the keyword stands alone and keeps the
	// redirection for itself — which is the null-command reading, and a
	// different command from the two above.
	for _, src := range []string{"function >f\n:\n", "function >f; :\n"} {
		f, err := Parse(src, bareKeyword())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if len(f.Stmts) != 2 {
			t.Fatalf("%q came to %d statements, want 2", src, len(f.Stmts))
		}
		fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*AnonFunc)
		if !ok || !fn.Bare {
			t.Errorf("%q came to %T, want a bare AnonFunc", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
			continue
		}
		if len(fn.Redirs) != 1 {
			t.Errorf("%q left %d redirections on the bare keyword, want 1", src, len(fn.Redirs))
		}
	}
}

func anonBodyTypeName(c Command) string {
	switch c.(type) {
	case *Group:
		return "*syntax.Group"
	case *SimpleCmd:
		return "*syntax.SimpleCmd"
	case *Subshell:
		return "*syntax.Subshell"
	}
	return "other"
}

func anonBodyRedirCount(c Command) int {
	switch x := c.(type) {
	case *Group:
		return len(x.Redirs)
	case *SimpleCmd:
		return len(x.Redirs)
	case *Subshell:
		return len(x.Redirs)
	}
	return -1
}
