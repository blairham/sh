// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	. "github.com/blairham/sh/syntax"
)

// SetDialect changes the grammar for what has not been read yet: a run-time
// option can decide, between one line and the next, whether a quantified
// group is a group. The front end calls it between NextLine calls; this pins
// the seam itself.
func TestSetDialectAppliesFromTheNextLine(t *testing.T) {
	src := "echo one\ncase ab in @(ab|cd)) echo hit;; esac\n"

	// Without the change the second line does not parse under the core.
	p := NewParser(src, Core())
	if _, ok := p.NextLine(); !ok {
		t.Fatal("the first line did not arrive")
	}
	if _, ok := p.NextLine(); ok && p.Err() == nil {
		t.Fatal("a quantified group parsed under the core")
	}

	// With it, the same second line is a group.
	p = NewParser(src, Core())
	if _, ok := p.NextLine(); !ok {
		t.Fatal("the first line did not arrive")
	}
	d := Core()
	d.ExtendedPattern = true
	p.SetDialect(d)
	line, ok := p.NextLine()
	if !ok || p.Err() != nil {
		t.Fatalf("the switched grammar refused the group: %v", p.Err())
	}
	if len(line.Stmts) != 1 {
		t.Errorf("got %d statements, want 1", len(line.Stmts))
	}
}
