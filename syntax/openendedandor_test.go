// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

func withOpenEndedAndOr() Dialect {
	d := Core()
	d.OpenEndedAndOr = true
	// The closing contexts a case below reaches need the constructs that
	// have them, and the brace group with no terminator before its `}` is
	// how the shell that takes an open-ended and-or is written.
	d.CloseBraceAlwaysReserved = true
	return d
}

// An and-or list may end with its operator where the flag is on. Measured
// 2026-09-07 on zsh 5.9.2 over a script file with a scratch HOME, ZDOTDIR and
// HISTFILE — every one of these runs there and every one of them is a syntax
// error in bash 5.3.15, the same binary as `sh`, bash 3.2.57, ksh93u+ and
// dash, each naming the closer it met.
//
//	$ zsh s.sh          # { : || \n }\n echo after
//	after
func TestAnAndOrMayEndWithItsOperatorWhereTheListCloses(t *testing.T) {
	for _, src := range []string{
		"{ : ||\n}\n",
		"{ : &&\n}\n",
		"{ : || }\n",
		"( : ||\n)\n",
		"( : || )\n",
		"if :; then : ||\nfi\n",
		"if :; then : ||\nelse :\nfi\n",
		"if :; then : ||\nelif :; then :\nfi\n",
		"while false; do : ||\ndone\n",
		"until :; do : ||\ndone\n",
		"for i in a; do : ||\ndone\n",
		"case x in x) : ||\n;; esac\n",
		"case x in x) : ||\nesac\n",
		"f() { : ||\n}\n",
		"x=$(: ||\n)\n",
	} {
		if _, err := Parse(src, withOpenEndedAndOr()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

// And the same lines without the flag, so the flag is what admits them rather
// than something else having changed. Core refuses every one.
func TestAnAndOrEndingWithItsOperatorIsRefusedWithoutTheFlag(t *testing.T) {
	d := Core()
	d.CloseBraceAlwaysReserved = true
	for _, src := range []string{
		"{ : ||\n}\n",
		"{ : &&\n}\n",
		"( : ||\n)\n",
		"if :; then : ||\nfi\n",
		"while false; do : ||\ndone\n",
		"case x in x) : ||\n;; esac\n",
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%q parsed under the core, want a refusal", src)
		}
	}
}

// The operator is *dropped*, not stood in for. That is the measured meaning —
// `false ||` answers 1 and `true &&` answers 0, so neither an implicit success
// nor an implicit failure fits — and it is why the interpreter needs nothing
// for this flag: what comes back is the left-hand side and there is no
// operator left in the tree.
func TestAnAbsentRightSideLeavesTheLeftSideAlone(t *testing.T) {
	f, err := Parse("{ echo one ||\n}\n", withOpenEndedAndOr())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("%d statements, want 1", len(f.Stmts))
	}
	group, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok || len(group.Cmds) != 1 {
		t.Fatalf("outer expression is %T, want a one-command pipeline", f.Stmts[0].Expr)
	}
	body, ok := group.Cmds[0].(*Group)
	if !ok {
		t.Fatalf("command is %T, want a *Group", group.Cmds[0])
	}
	if len(body.List) != 1 {
		t.Fatalf("%d statements in the group, want 1", len(body.List))
	}
	if be, ok := body.List[0].Expr.(*BinaryExpr); ok {
		t.Fatalf("the group holds a %v BinaryExpr, want the left-hand side alone", be.Op)
	}
}

// The pipeline does not take it, in any shell, and the flag must not widen to
// it: measured on zsh 5.9.2, all four of these are refused there and the
// refusal names the token found.
//
//	$ zsh -n s.sh       # ( true | )
//	s.sh:1: parse error near `)'
func TestAnOpenEndedPipelineIsStillRefused(t *testing.T) {
	for _, tc := range []struct{ src, token string }{
		{"( : | )\n", ")"},
		{"{ : |\n}\n", "}"},
		{"{ : | }\n", "}"},
		{"if :; then : |\nfi\n", "fi"},
		{"while false; do : |\ndone\n", "done"},
	} {
		_, err := Parse(tc.src, withOpenEndedAndOr())
		se, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: got %v, want a *syntax.Error", tc.src, err)
			continue
		}
		if se.Token != tc.token {
			t.Errorf("%q: blamed %q, want %q", tc.src, se.Token, tc.token)
		}
	}
}

// A terminator does not close the list for this purpose. `&` and a bare
// `case` terminator are parse errors after a dangling operator in the shell
// that has the flag, so they are not among the tokens that may stand there —
// and a `;` is #1142's question and not this flag's.
//
//	$ zsh -n s.sh       # true &&\n&\necho mark
//	s.sh:2: parse error near `&'
func TestATerminatorDoesNotEndAnOpenEndedAndOr(t *testing.T) {
	for _, tc := range []struct{ src, token string }{
		{": &&\n&\n", "&"},
		{": || & :\n", "&"},
		{": &&\n;;\n", ";;"},
		{": && ; :\n", ";"},
		{": && | :\n", "|"},
		{": && && :\n", "&&"},
	} {
		_, err := Parse(tc.src, withOpenEndedAndOr())
		se, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: got %v, want a *syntax.Error", tc.src, err)
			continue
		}
		if se.Token != tc.token {
			t.Errorf("%q: blamed %q, want %q", tc.src, se.Token, tc.token)
		}
	}
}

// A stop word with nothing open is still wrong, flag or no flag: the and-or
// gives up its operator, and then the line has a `fi` in it that closes
// nothing. Measured — zsh refuses `echo a && fi` and takes `if x; then echo a
// || fi`, which is the pair that says the flag is about the list closing and
// not about the word.
func TestAStopWordWithNothingOpenIsStillRefused(t *testing.T) {
	for _, tc := range []struct{ src, token string }{
		{"echo a && fi\n", "fi"},
		{"echo a || done\n", "done"},
		{": &&\n}\n", "}"},
	} {
		_, err := Parse(tc.src, withOpenEndedAndOr())
		se, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: got %v, want a *syntax.Error", tc.src, err)
			continue
		}
		if se.Token != tc.token {
			t.Errorf("%q: blamed %q, want %q", tc.src, se.Token, tc.token)
		}
	}
}

// Input that ran out after an and-or operator is unfinished rather than
// wrong, and the flag does not change that. It is the one case the
// token-naming path must not take, because there is no token to name — and it
// is what a continuation prompt is drawn from. Measured through a pty: zsh,
// bash and ksh93 all draw PS2 for a line ending on `&&`, so the answer at a
// terminal is the same in every column and the route is what the script case
// turns on (see [Dialect.OpenEndedAndOr]).
func TestInputEndingOnAnAndOrOperatorIsUnfinished(t *testing.T) {
	for _, d := range []Dialect{Core(), withOpenEndedAndOr()} {
		for _, src := range []string{": &&\n", ": ||\n", ": &&", "{ : ||\n"} {
			_, err := Parse(src, d)
			se, ok := err.(*Error)
			if !ok {
				t.Errorf("%q: got %v, want a *syntax.Error", src, err)
				continue
			}
			if se.Kind != ErrUnterminated {
				t.Errorf("%q: kind %v, want ErrUnterminated", src, se.Kind)
			}
		}
	}
}

// An and-or with nothing after it names the token it found, not the operator.
// That is what every shell in the panel that refuses these does, and it was
// wrong in every dialect: the substrate invented "expected a command after
// &&" — a sentence no shell writes, blaming a token that by then is read and
// was never the problem. #1115 fixed exactly this for the bar and left the
// and-or saying it.
func TestAnAndOrWithNoCommandNamesTheToken(t *testing.T) {
	for _, tc := range []struct {
		src, token string
		class      TokenClass
	}{
		{"a && ; b", ";", ClassOperator},
		{"a || ; b", ";", ClassOperator},
		{"a && & b", "&", ClassOperator},
		{"a && | b", "|", ClassOperator},
		{"a && && b", "&&", ClassOperator},
		{"a || || b", "||", ClassOperator},
		{"a && )", ")", ClassOperator},
		// A command may begin after `&&`, so a reserved word standing there
		// is reserved. The class travels because one dialect names it rather
		// than the token, and it names an *ordinary* word — so getting this
		// wrong turns `"fi" unexpected` into `word unexpected` there and
		// nowhere else.
		{"a && fi", "fi", ClassReserved},
		{"a || done", "done", ClassReserved},
		{"a && esac", "esac", ClassReserved},
	} {
		_, err := Parse(tc.src, Core())
		se, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: got %v, want a *syntax.Error", tc.src, err)
			continue
		}
		if se.Kind != ErrUnexpected {
			t.Errorf("%q: kind %v, want ErrUnexpected", tc.src, se.Kind)
		}
		if se.Token != tc.token {
			t.Errorf("%q: blamed %q, want %q", tc.src, se.Token, tc.token)
		}
		if se.Class != tc.class {
			t.Errorf("%q: class %v, want %v", tc.src, se.Class, tc.class)
		}
	}
}

// The openers below the operator are given back when the list closes without
// it, so nothing is left waiting for a command that is no longer coming.
//
// This is the only observable the accept path has besides the tree, and it is
// what a continuation prompt is drawn from — so a parse that forgot it would
// be correct in every other respect and wrong on the one surface a person
// sees. Measured on zsh 5.9.2 through a pty with PS2='[%_]': after
// `case x in x) : ||` the prompt is `[case cmdor]`, and once `;;` arrives it is
// `[case]` — the operator is gone. With a second arm dangling as well it says
// `case cmdor` once and not twice.
func TestTheOperatorIsGivenBackWhenItsListCloses(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		// The arm closed and took the `||` with it; the `case` is all that
		// is still open.
		{"case x in x) : ||\n;;\n", []string{"case"}},
		// A second arm dangling: one `||` waiting, not two.
		{"case x in x) : ||\n;; y) : ||\n", []string{"case", "||"}},
		// And the plain control — an operator whose command has not arrived
		// is still recorded, which is what the rows above are the absence of.
		{"case x in x) : ||\n", []string{"case", "||"}},
	} {
		p := NewParser(tc.src, withOpenEndedAndOr())
		p.Parse()
		var got []string
		for _, o := range p.Open() {
			got = append(got, o.Word)
		}
		if len(got) != len(tc.want) {
			t.Errorf("%q: open %v, want %v", tc.src, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%q: open %v, want %v", tc.src, got, tc.want)
				break
			}
		}
	}
}
