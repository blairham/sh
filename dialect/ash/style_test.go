// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/syntax"
)

// This shell's answers to the layout questions. Every one is the core's, and
// labeled a default rather than evidence: there is no ash on the machine whose
// scripts could be counted. See docs/spec/style.md.
func TestStyleAnswers(t *testing.T) {
	s := ash.Style()
	if s.Indent != "  " {
		t.Errorf("Indent = %q, want two spaces", s.Indent)
	}
	if !s.ThenOnHeaderLine || !s.DoOnHeaderLine {
		t.Errorf("then/do = %v/%v, want both on the header's line", s.ThenOnHeaderLine, s.DoOnHeaderLine)
	}
	if s.MaxBlankLines != 1 {
		t.Errorf("MaxBlankLines = %d, want 1", s.MaxBlankLines)
	}
	// The field that discriminates, and this shell is not the one it
	// discriminates for: only zsh parses a brace-spelled body.
	if s.BraceShortForm != syntax.ExpandShortForm {
		t.Errorf("BraceShortForm = %v, want expand", s.BraceShortForm)
	}
}
