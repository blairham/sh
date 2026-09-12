// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// shortLoop is the core plus the short forms, which is the grammar the short
// body belongs to. The dialect flag is named rather than a shell.
func shortLoop() syntax.Dialect {
	d := syntax.Core()
	d.ShortForm = true
	d.EmptyCompoundBody = true
	return d
}

// A loop whose condition is empty leaves the parse sitting on whatever stood
// where the condition should have been, and that token is the short body's to
// refuse. It used to be nobody's: the empty condition was allowed, the short
// body read no statement and returned quietly, and the first thing to object
// was the stop word two tokens later — so `while & do :; done` was refused at
// the `do`.
func TestAShortBodyRefusesTheTokenThatIsThere(t *testing.T) {
	for _, src := range []string{
		"while & do :; done\n",
		"until & do :; done\n",
		"while |& do :; done\n",
	} {
		_, err := syntax.Parse(src, shortLoop())
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%q: err = %v, want a parse failure", src, err)
			continue
		}
		if se.Token == "do" {
			t.Errorf("%q named the stop word, want the token in front of it", src)
		}
		if se.Pos.Col != 7 {
			t.Errorf("%q: refused at column %d, want 7 — the operator itself", src, se.Pos.Col)
		}
	}
}

// And the short body still reads the one statement it is for, so the refusal
// above is not a loop that stopped taking bodies.
func TestAShortBodyStillTakesItsStatement(t *testing.T) {
	for _, src := range []string{
		"while false; echo x\n",
		"for i in a b; echo $i\n",
	} {
		if _, err := syntax.Parse(src, shortLoop()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
