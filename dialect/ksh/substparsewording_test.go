// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A token the grammar refuses inside `$( … )` is worded by this dialect and
// located in the script, rather than printed as the parser's own coordinates.
//
// The body of a command substitution is kept raw by the lexer and parsed at
// expansion time, so its failure arrives at run time and not at the front
// end. That is this shell's own shape — measured 2026-09-12 on ksh93u+
// 2012-08-01, `echo one` / `echo $(if; then :; fi)` / `echo two` in a script
// file answers `one`, then
//
//	c.sh: line 2: syntax error at line 2: `;' unexpected
//
// at status 3, and `ksh -n` on the same file says nothing at all. Both halves
// of that are already right here; what was wrong was the sentence in the
// middle, which came out as `c.sh: line 2: 1:3: ";" unexpected` — a
// *syntax.Error rendered with %v, so a reader was handed the parser's line
// and column inside a message that had already named the right line of the
// file (#2460).
//
// This dialect is where the fix is worth pinning, for two reasons. Its
// wording writes the line *into* the sentence, so a body located from its own
// first line rather than from the script disagrees with the prefix beside it
// and the test can see it; and the sibling helper that reads the other two
// substitution spellings has asked the dialect for its wording since it was
// written, which is what made this one a single site left behind rather than
// a decision.
func TestARefusedSubstitutionBodyIsWordedByTheDialect(t *testing.T) {
	src := "echo one\necho $(if; then :; fi)\necho two\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Name: "c.sh", Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	const want = "one\nc.sh: line 2: syntax error at line 2: `;' unexpected\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	// Named separately from the comparison above so a future change to the
	// wording cannot quietly reintroduce the thing this is about: a position
	// in the parser's coordinates is never something to print at a user.
	if strings.Contains(out, "1:3:") {
		t.Errorf("output = %q, want no raw parser position in it", out)
	}
	if st != 3 {
		t.Errorf("status = %d, want 3", st)
	}
}
