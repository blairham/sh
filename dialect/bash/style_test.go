// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// This shell's answers to the layout questions. Every one is the core's; see
// docs/spec/style.md for what was measured and what was merely defaulted.
func TestStyleAnswers(t *testing.T) {
	s := bash.Style()
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
	// discriminates for: no dialect but zsh parses a brace-spelled body, so
	// preserving one here would be preserving something unreachable.
	if s.BraceShortForm != syntax.ExpandShortForm {
		t.Errorf("BraceShortForm = %v, want expand", s.BraceShortForm)
	}
}
