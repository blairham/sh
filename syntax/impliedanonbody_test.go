// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The bare `function` keyword takes the next command as a body it was given
// no brackets for (#5078).
//
// Written against the grammar flag rather than a shell, as everything in this
// package is. The behavior it produces is graded in dialect/zsh; what these
// rows assert is the *tree*, because that is where the two readings differ
// and the printed bytes often do not: a body that prints something prints the
// same thing whether it ran inside the call or after it.
func TestTheKeywordTakesTheNextCommandAsAnImpliedBody(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a simple command", "function\n:\n"},
		{"over a semicolon", "function; :\n"},
		{"over a blank line", "function\n\n:\n"},
		{"over a comment", "function\n# why\n:\n"},
		{"a loop", "function\nfor i in a; do :; done\n"},
		{"another bare keyword", "function\nfunction\n:\n"},
		// A word that merely begins with a reserved one is a command, which
		// is what says the peek reads the word to its end before judging it.
		{"a word that starts like a stop word", "function\ndonefile\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, bareKeyword())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			if len(f.Stmts) != 1 {
				t.Fatalf("%d statements, want 1: the command under the keyword is the body", len(f.Stmts))
			}
			fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*AnonFunc)
			if !ok {
				t.Fatalf("came to %T, want an AnonFunc", f.Stmts[0].Expr.(*Pipeline).Cmds[0])
			}
			if fn.Bare {
				t.Error("Bare is true, so the command under the keyword was left standing on its own")
			}
			if g, ok := fn.Body.(*Group); !ok || len(g.List) != 1 {
				t.Errorf("body came to %T, want a group of one statement", fn.Body)
			}
			// And the flag is what took it: the same text with only
			// BareFunctionKeyword off is a refusal, because the keyword
			// needs a name there.
			if _, err := Parse(tc.src, keywordNoBare()); err == nil {
				t.Errorf("%q parsed without the flag, so the reading is not the flag's", tc.src)
			}
		})
	}
}

// Where the keyword has nothing to take, it stands alone — and every one of
// these is a place where reading on would have taken something that is not a
// command.
func TestTheKeywordStandsAloneWhereTheNextThingIsNotACommand(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		stmts     int
		term      Terminator
	}{
		{"the end of the input", "function\n", 1, TerminatedByNewline},
		{"a closing brace", "{ function\n}\n", 1, TerminatedByNewline},
		{"a subshell's parenthesis", "( function\n)\n", 1, TerminatedByNewline},
		{"a loop's `done`", "for i in 1; do function; done\n", 1, TerminatedBySemicolon},
		{"an arm terminator", "case x in x) function ;; esac\n", 1, TerminatedByNothing},
		{"an `&&` it did not end before", "function && :\n", 1, TerminatedByNewline},
		{"a pipe it did not end before", "function | cat\n", 1, TerminatedByNewline},
		// A redirection ends the keyword's own command, so the command on
		// the next line is the script's and not a body: two statements.
		{"a redirection, and the next line is its own", "function >f\n:\n", 2, TerminatedByNewline},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, bareKeyword())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			if len(f.Stmts) != tc.stmts {
				t.Fatalf("%d statements, want %d", len(f.Stmts), tc.stmts)
			}
			fn, holder := firstAnonFunc(t, f)
			if !fn.Bare {
				t.Errorf("%q came to a body-taking call, want the bare form", tc.src)
			}
			// The terminator the keyword's own statement kept, which is the
			// level the reach is decided at and the only one it shows on.
			//
			// This engine looks ahead before it commits, so a keyword with
			// nothing to take leaves the `;` or the newline where it was.
			// Reading on and backing out would consume it, and **nothing
			// else can tell**: the statement runs the same, the construct
			// around it closes the same, and the printer writes the same
			// bytes — measured over sixty-five behavior rows and sixteen
			// formatter rows with the lookahead removed, all of which
			// agreed. So this is where that lookahead is graded, and
			// without these it is code no row can fail on.
			if holder.Term != tc.term {
				t.Errorf("%q left the statement terminated by %v, want %v — the keyword read past it",
					tc.src, holder.Term, tc.term)
			}
		})
	}
}

// firstAnonFunc digs the anonymous function out of the first statement,
// whichever side of an operator and inside whichever construct it was written
// in. Only the shapes the rows above use are descended into, which is what
// keeps it a test helper rather than a second walker.
// The statement it was found in comes back with it, because the terminator
// that statement kept is what grades the keyword's lookahead.
func firstAnonFunc(t *testing.T, f *File) (*AnonFunc, *Stmt) {
	t.Helper()
	var found *AnonFunc
	var holder *Stmt
	var walkExpr func(Expr)
	var walkStmts func([]*Stmt)
	walkCmd := func(c Command) {
		switch x := c.(type) {
		case *AnonFunc:
			if found == nil {
				found = x
			}
		case *Group:
			walkStmts(x.List)
		case *Subshell:
			walkStmts(x.List)
		case *ForClause:
			walkStmts(x.Body)
		case *CaseClause:
			for _, it := range x.Items {
				walkStmts(it.Body)
			}
		}
	}
	walkExpr = func(e Expr) {
		switch x := e.(type) {
		case *Pipeline:
			for _, c := range x.Cmds {
				walkCmd(c)
			}
		case *BinaryExpr:
			walkExpr(x.X)
			walkExpr(x.Y)
		}
	}
	walkStmts = func(list []*Stmt) {
		for _, st := range list {
			was := found
			walkExpr(st.Expr)
			if was == nil && found != nil {
				holder = st
			}
		}
	}
	holder = f.Stmts[0]
	walkExpr(f.Stmts[0].Expr)
	if found == nil {
		t.Fatalf("no AnonFunc in %#v", f.Stmts[0].Expr)
	}
	return found, holder
}

// The implied body prints back as the bracketed spelling of the same program,
// and reads back as the same tree.
//
// That is the license for holding it as a group of one statement: the two
// spellings are one program, measured — `function` over
// `typeset y=1 && print -r -- "[$y]"` writes the same bytes as
// `function { typeset y=1 && print -r -- "[$y]" }`, and neither leaves the
// name behind. So the brackets this form leaves out carry no fact for the
// tree to record, and printing them back is the same normalization the short
// `repeat` spelling already takes.
func TestTheImpliedBodyPrintsBackAsTheBracketedSpelling(t *testing.T) {
	for _, src := range []string{
		"function\n:\n",
		"function; :\n",
		"function\ntrue && false\n",
	} {
		f, err := Parse(src, bareKeyword())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		out := Print(f)
		back, err := Parse(out, bareKeyword())
		if err != nil {
			t.Fatalf("%q printed %q, which will not reparse: %v", src, out, err)
		}
		if len(back.Stmts) != 1 {
			t.Fatalf("%q printed %q, which reads back as %d statements", src, out, len(back.Stmts))
		}
		fn, ok := back.Stmts[0].Expr.(*Pipeline).Cmds[0].(*AnonFunc)
		if !ok || fn.Bare {
			t.Errorf("%q printed %q, which reads back as %T", src, out, back.Stmts[0].Expr.(*Pipeline).Cmds[0])
			continue
		}
		g, ok := fn.Body.(*Group)
		if !ok || len(g.List) != 1 {
			t.Errorf("%q printed %q, whose body reads back as %T", src, out, fn.Body)
		}
	}
}
