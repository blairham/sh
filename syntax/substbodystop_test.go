// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A word that ends the read of a `$( )` body with nothing to close is refused
// as one that stopped the read, and what the read was looking for is the
// closer; a token the grammar refused where it stood is neither (#6319).
func TestAStopWordInASubstitutionBodyExpectsTheCloser(t *testing.T) {
	for _, c := range []struct {
		name, src string
		stopped   bool
	}{
		{"read with the line", "v=$(esac", true},
		{"a body read on its own", "", true},
		{"a refused token", "v=$(echo a; ;", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var p *syntax.Parser
			if c.src == "" {
				p = syntax.NewParser("fi", syntax.Core())
				p.InsideASubstitution()
				p.InsideProgramParentheses()
			} else {
				p = syntax.NewParser(c.src, syntax.Core())
			}
			p.Parse()
			var se *syntax.Error
			if !errors.As(p.Err(), &se) {
				t.Fatalf("no refusal: %v", p.Err())
			}
			if se.BodyRefusal != nil {
				se = se.BodyRefusal
			}
			if se.StoppedTheRead != c.stopped || (se.Expected == ")") != c.stopped {
				t.Errorf("stopped %v expected %q, want stopped %v", se.StoppedTheRead, se.Expected, c.stopped)
			}
		})
	}
}

// And in the dialect that ends a backquoted body there, the word ends it
// quietly and what follows it is dropped; the `;` is still refused.
// See Dialect.BackquoteBodyEndsAtAStopWord.
func TestABackquotedBodyEndsAtAStopWordWhereTheDialectSays(t *testing.T) {
	read := func(src string, ends bool) (*syntax.File, error) {
		d := syntax.Core()
		d.BackquoteBodyEndsAtAStopWord = ends
		p := syntax.NewParser(src, d)
		p.InsideASubstitution()
		return p.Parse(), p.Err()
	}
	f, err := read("echo a; fi; echo b", true)
	if err != nil || len(f.Stmts) != 1 {
		t.Errorf("with the flag: %d statements, error %v; want `echo a` alone", len(f.Stmts), err)
	}
	if _, err := read("echo a; fi; echo b", false); err == nil {
		t.Error("without the flag the `fi` was not refused")
	}
	if _, err := read("echo a; ;", true); err == nil {
		t.Error("with the flag the `;` was not refused")
	}
}
