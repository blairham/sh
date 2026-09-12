// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

func withOneSeparator() Dialect {
	d := Core()
	d.SeparatorWhereACommandBelongs = OneSeparatorExceptAfterABarOrBeforeACondition
	d.AbsentAndOrOperandIsAnEmptyCommand = true
	return d
}

func withAnySeparator() Dialect {
	d := Core()
	d.SeparatorWhereACommandBelongs = AnySeparatorWhereACommandBelongs
	d.OpenEndedAndOr = true
	d.CloseBraceAlwaysReserved = true
	return d
}

// A `;` written where a command belongs is stepped over. One rule, and every
// shape below is it seen in a different position — which is the finding, and
// the reason this is not a flag per control operator.
//
// Measured 2026-09-07 over a script file with a scratch HOME, ZDOTDIR and
// HISTFILE. dash, bash 5.3, bash 3.2 and bash-as-`sh` refuse all of them.
func TestASeparatorWhereACommandBelongsIsSteppedOver(t *testing.T) {
	for _, src := range []string{
		"; echo two\n",
		"echo one ; ; echo two\n",
		"echo one & ; echo two\n",
		"echo one ; ;\n",
		"echo one && ; echo two\n",
		"echo one || ; echo two\n",
		"echo one ||\n; echo two\n",
		"{ echo one ; ; echo two ; }\n",
	} {
		for name, d := range map[string]Dialect{
			"one": withOneSeparator(), "any": withAnySeparator(),
		} {
			if _, err := Parse(src, d); err != nil {
				t.Errorf("%s: %q: %v", name, src, err)
			}
		}
		if _, err := Parse(src, Core()); err == nil {
			t.Errorf("%q parsed under the core, want a refusal", src)
		}
	}
}

// The `;` is **skipped**, not stood in for: the command after it becomes the
// operator's own right-hand side. That is what `false` shows and `true` hides
// — `false || ; echo two` prints `two` and `true || ; echo two` prints
// nothing — and it is why this is a separator rule rather than an absent
// operand one.
func TestTheSeparatorIsSkippedAndNotAnOperand(t *testing.T) {
	for _, d := range []Dialect{withOneSeparator(), withAnySeparator()} {
		f, err := Parse("false || ; echo two\n", d)
		if err != nil {
			t.Fatal(err)
		}
		if len(f.Stmts) != 1 {
			t.Fatalf("%d statements, want 1 — the `;` ended one", len(f.Stmts))
		}
		be, ok := f.Stmts[0].Expr.(*BinaryExpr)
		if !ok {
			t.Fatalf("expression is %T, want a *BinaryExpr", f.Stmts[0].Expr)
		}
		if be.Op != TokOrOr {
			t.Errorf("operator %v, want ||", be.Op)
		}
		pl, ok := be.Y.(*Pipeline)
		if !ok || len(pl.Cmds) != 1 {
			t.Fatalf("right-hand side is %T, want a one-command pipeline", be.Y)
		}
		cmd, ok := pl.Cmds[0].(*SimpleCmd)
		if !ok || len(cmd.Args) != 2 {
			t.Fatalf("right-hand side is not the two-word `echo two`: %#v", pl.Cmds[0])
		}
	}
}

// One dialect steps over a single separator and the other over as many as are
// written. Measured: `a || ; ; b` is “ `;' unexpected “ in ksh93 and runs in
// zsh, and the same for `echo one | ; ; cat -n`.
func TestHowManySeparatorsAreSteppedOver(t *testing.T) {
	for _, src := range []string{
		"echo one || ; ; echo two\n",
		"; ; echo two\n",
	} {
		if _, err := Parse(src, withOneSeparator()); err == nil {
			t.Errorf("%q parsed where only one separator may be stepped over", src)
		}
		if _, err := Parse(src, withAnySeparator()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

// And one of them will not step over a bar's, which is the discriminating
// probe: ksh93 takes `a || ; b` and `a |& ; b` and refuses `a | ; b`, so the
// bar is a separate position and not one rule about control operators.
func TestASeparatorAfterABarIsItsOwnQuestion(t *testing.T) {
	for _, src := range []string{
		"echo one | ; cat\n",
		"echo one |\n; cat\n",
	} {
		_, err := Parse(src, withOneSeparator())
		se, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: got %v, want a *syntax.Error", src, err)
		} else if se.Token != ";" {
			t.Errorf("%q: blamed %q, want %q", src, se.Token, ";")
		}
		if _, err := Parse(src, withAnySeparator()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

// Where a separator was stepped over and the list ended there, one dialect
// puts a command that does nothing in its place, so the and-or has a
// right-hand side that succeeds. The other drops the operator instead
// ([Dialect.OpenEndedAndOr]), and the two answer `false || ;` differently —
// 0 against 1 — which is why they are separate fields.
func TestAnAbsentOperandAfterASeparatorIsAnEmptyCommand(t *testing.T) {
	for _, src := range []string{
		"false || ;\n",
		"{ false || ; }\n",
		"( false || ; )\n",
		"if :; then false || ; fi\n",
	} {
		d := withOneSeparator()
		d.CloseBraceAlwaysReserved = true
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		found := firstAndOr(f.Stmts)
		if found == nil {
			t.Fatalf("%q: no and-or in the tree; the operator was dropped", src)
		}
		pl, ok := found.Y.(*Pipeline)
		if !ok || len(pl.Cmds) != 1 {
			t.Fatalf("%q: right-hand side is %T", src, found.Y)
		}
		cmd, ok := pl.Cmds[0].(*SimpleCmd)
		if !ok {
			t.Fatalf("%q: right-hand side is %T, want a *SimpleCmd", src, pl.Cmds[0])
		}
		if len(cmd.Args) != 0 || len(cmd.Assigns) != 0 || len(cmd.Redirs) != 0 {
			t.Errorf("%q: the command standing in is not empty: %#v", src, cmd)
		}
	}
}

// The empty command stands in only where the list *ended* there. A second
// separator is not the end of anything, so it is still refused and still
// names itself — which is what the shell that has this does.
func TestTheEmptyCommandDoesNotStandInForASecondSeparator(t *testing.T) {
	_, err := Parse("false || ; ; echo two\n", withOneSeparator())
	se, ok := err.(*Error)
	if !ok {
		t.Fatalf("got %v, want a *syntax.Error", err)
	}
	if se.Token != ";" {
		t.Errorf("blamed %q, want %q", se.Token, ";")
	}
}

// firstAndOr digs the and-or out of whatever compound the case wrapped it in,
// so one assertion covers the brace group, the subshell and the `if`.
func firstAndOr(stmts []*Stmt) *BinaryExpr {
	for _, st := range stmts {
		switch e := st.Expr.(type) {
		case *BinaryExpr:
			return e
		case *Pipeline:
			for _, c := range e.Cmds {
				switch b := c.(type) {
				case *Group:
					if got := firstAndOr(b.List); got != nil {
						return got
					}
				case *Subshell:
					if got := firstAndOr(b.List); got != nil {
						return got
					}
				case *IfClause:
					if got := firstAndOr(b.Then); got != nil {
						return got
					}
				}
			}
		}
	}
	return nil
}

// A newline *after* the separator is the dialect's own question, and the two
// shells answer it differently. It is the row this nearly got wrong: stepping
// over the newline for both is a one-line convenience that changes what
// `true || ; ⏎ echo two` means.
//
//	$ ksh s.sh          # true || ; ⏎ echo two
//	two
//	$ zsh s.sh
//	(nothing)
//
// zsh reads on, so `echo two` is the right-hand side and the `true`
// short-circuits past it — one statement. ksh93 stops at the newline, so the
// and-or ends with nothing on its right and `echo two` is the statement after
// it — two statements. The count is what says which happened.
func TestANewlineAfterTheSeparatorIsTheDialectSAnswer(t *testing.T) {
	const src = "true || ;\necho two\n"
	f, err := Parse(src, withOneSeparator())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stmts) != 2 {
		t.Errorf("stepping over one: %d statements, want 2 — the newline ended the and-or", len(f.Stmts))
	}
	f, err = Parse(src, withAnySeparator())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stmts) != 1 {
		t.Errorf("stepping over any: %d statements, want 1 — `echo two` is the right-hand side", len(f.Stmts))
	}
	// And the same newline between two *statements* is an ordinary
	// terminator in both, which is the control: nothing about a `;` changes
	// what a newline does where a list separator already belongs.
	for name, d := range map[string]Dialect{
		"one": withOneSeparator(), "any": withAnySeparator(),
	} {
		f, err := Parse("true ; ;\necho two\n", d)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(f.Stmts) != 2 {
			t.Errorf("%s: %d statements, want 2", name, len(f.Stmts))
		}
	}
}

// The rule reaches the *first* thing in a compound body as well as the space
// between two statements, which is a separate call site and was the gap a
// surviving mutant found: removing the skip at the head of a list left every
// test passing, because every case wrote the `;` after something.
//
// Measured 2026-09-07 — ksh93 and zsh print `two` for all five, dash and all
// three bash columns refuse all five.
//
//	$ ksh s.sh          # { ; echo two; }
//	two
func TestASeparatorAtTheHeadOfACompoundBody(t *testing.T) {
	for _, src := range []string{
		"{ ; echo two; }\n",
		"( ; echo two )\n",
		"if :; then ; echo two; fi\n",
		"while :; do ; echo two; break; done\n",
		"case x in x) ; echo two;; esac\n",
	} {
		for name, d := range map[string]Dialect{
			"one": withOneSeparator(), "any": withAnySeparator(),
		} {
			if _, err := Parse(src, d); err != nil {
				t.Errorf("%s: %q: %v", name, src, err)
			}
		}
		if _, err := Parse(src, Core()); err == nil {
			t.Errorf("%q parsed under the core, want a refusal", src)
		}
	}
}

// The openers below the operator are given back when an empty command stands
// in for its right-hand side, so nothing is left waiting for a command that
// is no longer coming.
//
// It is the same miss #1174's survivor found on the other branch, and it
// survives every parse assertion: the tree is identical either way and only
// what the parser reports as still *open* changes. That reaches a rendered
// diagnostic here — the dialect with this flag names the innermost unclosed
// thing in an `unmatched` message — so a stale entry would put an operator
// where a keyword belongs.
//
// What the real shell names in that message is a separate question and is
// **not** asserted: measured, ksh93 answers `if :; then false || ;` with
// “ `;' unmatched “ where it answers `if :; then :` with “ `then'
// unmatched “, so the `;` that admitted the empty command becomes the thing
// it reports. That is recorded as a follow-up rather than guessed at from a
// handful of probes. What is certain either way is that the resolved `||` is
// not it, which is what this pins.
func TestTheOperatorIsGivenBackWhenAnEmptyCommandStandsIn(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{"case x in x) false || ;\n;;\n", []string{"case"}},
		{"{ false || ;\n", []string{"{"}},
		{"if :; then false || ;\n", []string{"if", "then"}},
		{"while :; do false || ;\n", []string{"while", "do"}},
		// Two arms dangling, so a fix that gave one back and not the rest
		// still fails.
		{"case x in x) false || ;\n;; y) false || ;\n", []string{"case"}},
	} {
		p := NewParser(tc.src, withOneSeparator())
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
