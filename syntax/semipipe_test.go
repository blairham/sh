// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// `;|` — zsh's spelling of `;;&`, in the grammar.
//
// Measured 2026-09-07, `env -i` with a scratch HOME, ZDOTDIR and HISTFILE,
// over a script file. Three arms, subject `b`, each arm echoing its own
// letter:
//
//	terminator  dash   bash 3.2  bash 5   bash as sh  ksh93  zsh
//	;;          B      B         B        B           B      B
//	;&          error  error     B star   B star      B star B star
//	;|          error  error     error    error       error  B star
//	;;&         error  error     B star   B star      error  error
//
// The last two rows are the finding: **`;|` and `;;&` are mutually
// exclusive**. zsh takes `;|` and refuses `;;&`; bash 4-and-later takes
// `;;&` and refuses `;|`; dash, bash 3.2 and ksh93 have neither. That is
// why this is a second flag beside CaseContinue and not a second value of
// it — no single flag could be given a value for zsh.

// withSemiPipe is Core plus the one flag under test.
func withSemiPipe() Dialect {
	d := Core()
	d.CaseContinuePipe = true
	return d
}

// TestSemiPipeTerminatesACaseArm — the terminator slot takes it, and the
// arm's Term records *which* spelling was written. Folding the two into one
// Kind would print a zsh script back as bash.
func TestSemiPipeTerminatesACaseArm(t *testing.T) {
	const src = "case b in\n  b) echo B ;|\n  *) echo star ;;\nesac"
	f, err := Parse(src, withSemiPipe())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("statements = %d, want 1", len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("statement is %T, want one command", f.Stmts[0].Expr)
	}
	c, ok := pipe.Cmds[0].(*CaseClause)
	if !ok {
		t.Fatalf("command is %T, want *CaseClause", pipe.Cmds[0])
	}
	if len(c.Items) != 2 {
		t.Fatalf("arms = %d, want 2", len(c.Items))
	}
	if c.Items[0].Term != TokSemiPipe {
		t.Errorf("first arm's Term = %v, want %v", c.Items[0].Term, TokSemiPipe)
	}
	if c.Items[1].Term != TokDSemi {
		t.Errorf("second arm's Term = %v, want %v", c.Items[1].Term, TokDSemi)
	}
	// The spelling survives to the printed form, which is the only way a
	// script read here can be written back for the shell it came from.
	if got := c.Items[0].Term.String(); got != ";|" {
		t.Errorf("Term.String() = %q, want %q", got, ";|")
	}
}

// TestSemiPipeOnTheLastArm — measured, zsh accepts it with no arm after it
// to test, and carries on past the `esac`.
func TestSemiPipeOnTheLastArm(t *testing.T) {
	const src = "case b in\n  b) echo B ;|\nesac\necho after"
	f, err := Parse(src, withSemiPipe())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Fatalf("statements = %d, want 2 — the case and the echo after it", len(f.Stmts))
	}
}

// TestSemiPipeEndsAListLikeTheOtherTerminators — an arm's body is a list,
// and `;|` has to be one of the tokens a list *ends* on, not merely one the
// arm's parser recognizes afterwards.
//
// The discriminating shape is #1142's: an and-or with no right-hand side.
// Measured 2026-09-07 on zsh 5.9.2 over a script file,
// `case b in b) true || ;| *) echo star ;; esac; echo done` prints `star`
// then `done` — the empty and-or is allowed, the `;|` closes the list, and
// the later pattern is still tested. The `;;` control prints `done` alone,
// which is what shows the `;|` did the continuing rather than the arm
// merely ending.
//
// Without this the terminator still parsed, because the arm's own switch
// reads it a moment later: the list ends at the `;|` for the same reason it
// ends at `;;`, and only a body that has *stopped early* can tell.
func TestSemiPipeEndsAListLikeTheOtherTerminators(t *testing.T) {
	// OpenEndedAndOr is the flag that allows the empty right-hand side; it
	// is zsh's, and named here beside the terminator because the shape needs
	// both. A test with only one of them measures nothing about the other.
	d := withSemiPipe()
	d.OpenEndedAndOr = true
	for _, src := range []string{
		`case b in b) true || ;| *) echo star ;; esac`,
		"case b in\n  b) echo one; true ||\n  ;| *) echo star ;;\nesac",
	} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
	// The control: the same shape on `;;`, which has always ended a list.
	// If this ever fails, the shape is wrong rather than the terminator.
	if _, err := Parse(`case b in b) true || ;; *) echo star ;; esac`, d); err != nil {
		t.Errorf("the `;;` control: %v", err)
	}
}

// TestSemiPipeIsRefusedWhereTheFlagIsOff — and refused at the `|`, not at
// the `;`, because the operator table falls back to `;` and then `|`: that
// is exactly what dash, bash and ksh93 lex, so the refusal lands where
// theirs does rather than being worded twice.
func TestSemiPipeIsRefusedWhereTheFlagIsOff(t *testing.T) {
	const src = "case b in\n  b) echo B ;|\n  *) echo star ;;\nesac"
	if _, err := Parse(src, Core()); err == nil {
		t.Fatal("core parsed `;|`, want a refusal")
	} else if !strings.Contains(err.Error(), "|") {
		t.Errorf("core refused with %q, want the `|` named", err)
	}
	// The flags are independent in both directions.
	if _, err := Parse("case b in\n  b) echo B ;;&\nesac", withSemiPipe()); err == nil {
		t.Fatal("the `;|` dialect parsed `;;&`, want a refusal — the two are exclusive")
	}
	d := Core()
	d.CaseContinue = true
	if _, err := Parse(src, d); err == nil {
		t.Fatal("the `;;&` dialect parsed `;|`, want a refusal — the two are exclusive")
	}
}

// TestSemiPipeIsOneOperatorEverywhere — the token is not confined to a
// `case` arm, because zsh's is not: `echo a ;| echo b` is “parse error
// near `;|'“ there, naming both bytes, where the other four name the bare
// `|`. So the refusal outside a case has to name the operator too, and it
// does because the lexer decides this and not the parser.
func TestSemiPipeIsOneOperatorEverywhere(t *testing.T) {
	_, err := Parse("echo a ;| echo b", withSemiPipe())
	if err == nil {
		t.Fatal("`;|` outside a case parsed, want a refusal")
	}
	if !strings.Contains(err.Error(), ";|") {
		t.Errorf("refused with %q, want the whole `;|` named", err)
	}
	// Two bytes with nothing between them, like `|&`: measured, zsh refuses
	// `; |` at the `|` rather than reading it as the terminator.
	_, err = Parse("case b in\n  b) echo B ; |\n  *) echo star ;;\nesac", withSemiPipe())
	if err == nil {
		t.Fatal("`; |` with a blank parsed as the terminator, want a refusal")
	}
	if strings.Contains(err.Error(), ";|") {
		t.Errorf("refused with %q, want the `|` named and not the operator", err)
	}
}
