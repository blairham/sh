// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// This shell's answers to the layout questions — and it is the only shell
// whose answers are not all the core's.
func TestStyleAnswers(t *testing.T) {
	s := zsh.Style()
	if s.Indent != "  " {
		t.Errorf("Indent = %q, want two spaces", s.Indent)
	}
	if !s.ThenOnHeaderLine || !s.DoOnHeaderLine {
		t.Errorf("then/do = %v/%v, want both on the header's line", s.ThenOnHeaderLine, s.DoOnHeaderLine)
	}
	if s.MaxBlankLines != 1 {
		t.Errorf("MaxBlankLines = %d, want 1", s.MaxBlankLines)
	}
}

// TestTheBraceSpellingIsKept is the one answer that differs from every other
// dialect, and the reason Style is a vector rather than a constant in the
// printer.
//
// This shell spells a body with braces — `if cond { … }`, `for f ( a b c )
// { … }` — and those parse to exactly the tree the keyword spelling parses
// to, so nothing but this field can say which one a formatter writes back.
// Measured: 317 `if … {`, 120 `} else {` and 61 `for … ( ) {` across 335 zsh
// scripts on the machine, and none in any other dialect.
func TestTheBraceSpellingIsKept(t *testing.T) {
	if got := zsh.Style().BraceShortForm; got != syntax.PreserveShortForm {
		t.Errorf("BraceShortForm = %v, want preserve — see docs/spec/style.md", got)
	}
}

// TestStyleDiffersFromTheCoreInExactlyOnePlace pins the size of the
// disagreement as well as its direction. A second field drifting away from
// the core without a measurement behind it should fail here first.
func TestStyleDiffersFromTheCoreInExactlyOnePlace(t *testing.T) {
	core, ours := syntax.CoreStyle(), zsh.Style()
	core.BraceShortForm = ours.BraceShortForm
	if core != ours {
		t.Errorf("zsh's style differs from the core's in more than the brace spelling:\ncore: %+v\nzsh:  %+v", core, ours)
	}
}
