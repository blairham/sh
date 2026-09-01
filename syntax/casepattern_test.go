// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// CasePatternAcceptsOperator is a grammar difference and not an error
// recovery, which is why it is tested by what *parses* rather than by what a
// failure says: `case a in & ) … esac` is a running script in the dialect
// that has it, and an arm that matches nothing.
func TestCasePatternAcceptsOperator(t *testing.T) {
	takes, refuses := syntax.POSIX(), syntax.POSIX()
	takes.CasePatternAcceptsOperator = true

	f, err := syntax.Parse("case a in & ) echo hit;; *) echo miss;; esac", takes)
	if err != nil {
		t.Fatalf("should parse where the flag is set: %v", err)
	}
	// The arm it opens has no patterns at all, which is what makes it match
	// nothing — not the operator, and not the empty string.
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok {
		t.Fatalf("got %T, want a pipeline", f.Stmts[0].Expr)
	}
	c, ok := pipe.Cmds[0].(*syntax.CaseClause)
	if !ok {
		t.Fatalf("got %T, want a case clause", pipe.Cmds[0])
	}
	if n := len(c.Items[0].Patterns); n != 0 {
		t.Errorf("the first arm has %d patterns, want none", n)
	}

	if _, err := syntax.Parse("case a in & ) echo hit;; esac", refuses); err == nil {
		t.Error("should not parse where the flag is not set")
	}

	// Only one operator, and only where the pattern list would start.
	for _, src := range []string{"case a in &a ) echo x;; esac", "case a in a& ) echo x;; esac"} {
		if _, err := syntax.Parse(src, takes); err == nil {
			t.Errorf("%q should not parse", src)
		}
	}
}

// A reserved word is only reserved where a command could begin.
//
// `esac` in the pattern position is an ordinary word, and the dialect that
// names a token's class says so — the difference is invisible until something
// asks for the class, which is why it went unnoticed until one case did.
func TestReservedWordsAreOnlyReservedWhereACommandCouldBegin(t *testing.T) {
	d := syntax.POSIX()
	d.CasePatternAcceptsOperator = true
	_, err := syntax.Parse("case a in a) echo x;;& esac", d)
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("got %v, want a *syntax.Error", err)
	}
	if se.Class != syntax.ClassWord {
		t.Errorf("esac where `)` belongs is class %v, want ClassWord", se.Class)
	}

	// In command position it is reserved, which is the other half.
	_, err = syntax.Parse("if true; echo x; fi", syntax.POSIX())
	if !errors.As(err, &se) {
		t.Fatalf("got %v, want a *syntax.Error", err)
	}
	if se.Class != syntax.ClassReserved {
		t.Errorf("fi in command position is class %v, want ClassReserved", se.Class)
	}
	if !strings.Contains(se.Msg, "fi") {
		t.Errorf("msg = %q, want it to name the token", se.Msg)
	}
}
