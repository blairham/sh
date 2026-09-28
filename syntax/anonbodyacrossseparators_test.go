// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A nameless function's body may stand on a later line, and the words after
// it are still the call's (#3778).
//
// Written against the grammar flag rather than a shell, as everything in this
// package is: bareKeyword() is the core with the anonymous function and the
// bare keyword both on, which is the one dialect where the two readings
// compete. See [Dialect.BareFunctionKeyword].
//
// The competition is the whole difficulty. The keyword standing alone is a
// command here, so `function` at the end of a line has a complete reading
// already — and taking it leaves the `{ … }` under it as an ordinary group,
// at which point the words after that group follow a closed command and there
// is nowhere for them to go. The reference keeps reading for a body first.
func TestANamelessFunctionsBodyMayStandOnALaterLine(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		args      int
	}{
		{"a newline between the two", "function\n{ :; } a b\n", 2},
		{"a blank line as well", "function\n\n{ :; } a b\n", 2},
		{"a comment line", "function\n# why\n{ :; } a b\n", 2},
		{"a comment after the keyword", "function # why\n{ :; } a b\n", 2},
		{"a semicolon", "function; { :; } a b\n", 2},
		{"a semicolon and a newline", "function;\n{ :; } a b\n", 2},
		{"a line continuation", "function \\\n{ :; } a b\n", 2},
		{"a subshell body", "function\n( : ) a b\n", 2},
		// And with no arguments at all, which is the row that looks as
		// though it needed nothing: it is a *call* either way rather than a
		// group, and only the node says so.
		{"no arguments, and still a call", "function\n{ :; }\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, bareKeyword())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			if len(f.Stmts) != 1 {
				t.Fatalf("%d statements, want 1: the body and its words are one command", len(f.Stmts))
			}
			cmd := f.Stmts[0].Expr.(*Pipeline).Cmds[0]
			fn, ok := cmd.(*AnonFunc)
			if !ok {
				t.Fatalf("came to %T, want an AnonFunc", cmd)
			}
			if fn.Bare {
				t.Error("Bare is true, so the body under the keyword was read as a separate group")
			}
			if len(fn.Args) != tc.args {
				t.Errorf("%d arguments, want %d", len(fn.Args), tc.args)
			}
		})
	}
}

// And the discriminator: what ends the keyword's command rather than
// separating it from the next one leaves the bare reading standing.
//
// Without these the rows above pass just as well against a parser that reads
// every `function` as reaching for the next brace group anywhere ahead of it,
// which would take `function >f` and `function && …` away from the readings
// they already have. Each of these is a refusal in the reference, measured a
// spelling at a time.
//
// The *token* is asserted and not merely the refusal, because a refusal on its
// own is not discriminating here. A parser that reached past an arm terminator
// still fails on the same input — it enters the nameless-function branch and
// then finds no body — so both readings refuse and only the wording separates
// them. The reference says `parse error near `echo'`, which is the body's
// first word standing where an arm's pattern should: the bare keyword was
// taken, the arm ended at the `;;`, and what follows opened a new one. A
// complaint about a missing body is the other reading saying so out loud.
func TestWhatEndsTheKeywordsCommandStopsTheReach(t *testing.T) {
	for _, tc := range []struct{ name, src, token string }{
		{"a redirection", "function >/dev/null\n{ echo hi; } a\n", "a"},
		{"an and-if", "function && { echo hi; } a\n", "a"},
		{"an or-if", "function || { echo hi; } a\n", "a"},
		{"an arm terminator", "case x in x) function;; { echo hi; } a\nesac\n", "echo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src, bareKeyword())
			if err == nil {
				t.Fatalf("%q parsed, want a refusal: the words have nowhere to go", tc.src)
			}
			e, ok := err.(*Error)
			if !ok {
				t.Fatalf("%q came to %T, want a parse error", tc.src, err)
			}
			if e.Kind != ErrUnexpected {
				t.Errorf("kind %v, want ErrUnexpected: the refusal is at a token and not over a body nobody wrote", e.Kind)
			}
			if e.Token != tc.token {
				t.Errorf("refused %q, want %q", e.Token, tc.token)
			}
		})
	}
}

// A word under the keyword is a *command* and not a name, and a brace group
// after that word is a refusal.
//
// The second discriminator, and the one that says the reach is a **body**
// position rather than a license to skip ahead: the reference refuses
// `function` ⏎ `foo { … }` on the `}`. `foo` was read as the command it looks
// like, and the brace group has nowhere to go behind it — which is a different
// sentence from "`foo` is this declaration's name", and the difference is what
// #5078 turned on. A word under the keyword *is* taken, as the body; it is the
// group behind it that cannot be.
func TestAWordUnderTheKeywordIsACommandAndAGroupBehindItIsARefusal(t *testing.T) {
	if _, err := Parse("function\nfoo { :; }\n", bareKeyword()); err == nil {
		t.Error("`function` then `foo { :; }` parsed, want a refusal")
	}
	// And the keyword with an ordinary command under it is **one** command:
	// the call and its body. This asserted two statements and a bare node
	// until #5078, which is the reading the reference does not have.
	f, err := Parse("function\n:\n", bareKeyword())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("%d statements, want 1: the command under the keyword is its body", len(f.Stmts))
	}
	fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*AnonFunc)
	if !ok {
		t.Fatalf("came to %T, want an AnonFunc", f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	if fn.Bare {
		t.Error("Bare is true, so the command under the keyword was left standing on its own")
	}
	g, ok := fn.Body.(*Group)
	if !ok || len(g.List) != 1 {
		t.Errorf("body came to %T, want a group of one statement", fn.Body)
	}
}

// The other spelling of the header reaches over a `;` too, which is what says
// the separator belongs to the position and not to the keyword.
func TestTheParenthesisedHeaderReachesOverASeparatorAsWell(t *testing.T) {
	f, err := Parse("() ; { :; } a b\n", bareKeyword())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*AnonFunc)
	if !ok {
		t.Fatalf("came to %T, want an AnonFunc", f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	if fn.Keyword {
		t.Error("Keyword is true, so the spelling that was written is lost")
	}
	if len(fn.Args) != 2 {
		t.Errorf("%d arguments, want 2", len(fn.Args))
	}
}
