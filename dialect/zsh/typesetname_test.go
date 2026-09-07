// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// An operand a declaration will not take, in this shell's two wordings.
//
// This shell tells the two shapes apart where the other two do not: a leading
// digit is `not an identifier` and anything else is `not valid in this
// context`. Both are fatal, which is the half a wording test cannot see — the
// line after the declaration is what says the script stopped.
//
// Measured 2026-09-07 against zsh 5.9.2 with a scratch HOME and `-f`.
func TestADeclarationOperandThatIsNotAName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset ':'; echo A`, "zsh:typeset:1: not valid in this context: :\n"},
		{`typeset 1x; echo A`, "zsh:typeset:1: not an identifier: 1x\n"},
		{`typeset 'a b'; echo A`, "zsh:typeset:1: not valid in this context: a b\n"},
		{`typeset ''; echo A`, "zsh:typeset:1: not valid in this context: \n"},
		{`typeset x-y; echo A`, "zsh:typeset:1: not valid in this context: x-y\n"},
		{`declare ':'; echo A`, "zsh:declare:1: not valid in this context: :\n"},
		{`declare 1x; echo A`, "zsh:declare:1: not an identifier: 1x\n"},
		// The third name, which calls itself by the name it was invoked
		// under — ksh93's calls itself `typeset` and this one does not.
		{`integer 1x; echo A`, "zsh:integer:1: not an identifier: 1x\n"},
		{`integer ':'; echo A`, "zsh:integer:1: not valid in this context: :\n"},
		// The name is judged and the *name* is reported, not the whole
		// operand: the two other shells quote `1x=v` back.
		{`typeset '1x=v'; echo A`, "zsh:typeset:1: not an identifier: 1x\n"},
		{`typeset ':=v'; echo A`, "zsh:typeset:1: not valid in this context: :\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", tc.src, out, st, tc.want)
		}
		if strings.Contains(out, "A") {
			t.Errorf("%s ran the line after the declaration: %q", tc.src, out)
		}
	}
}

// A special parameter is a name to a declaration here, which is the reason
// the strictness is a value and not a rule: `typeset 0` is no complaint at
// all in this shell and is one in the other two.
func TestADeclarationTakesASpecialParameter(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{`typeset 0`, `typeset '?'`, `typeset '#'`, `typeset -- -`} {
		out, st := runZsh(t, dir, src)
		if st != 0 || strings.Contains(out, "not ") {
			t.Errorf("%s = %q (status %d), want no complaint at 0", src, out, st)
		}
	}
	// And a positional parameter is not one, which is what tells the two
	// apart: `1` is refused where `0` is taken.
	out, st := runZsh(t, dir, `typeset 1; echo A`)
	if want := "zsh:typeset:1: not an identifier: 1\n"; out != want || st != 1 {
		t.Errorf("typeset 1 = %q (status %d), want %q at 1", out, st, want)
	}
}

// The tables the refusals above are read out of, asserted as tables.
//
// A dialect is data, so the entries are worth naming: a missing one falls
// through to the substrate's default wording, which is bash's sentence, and
// nothing above would catch that for a builtin no test happened to cover.
func TestTheDeclarationBadNameWordingsAreSet(t *testing.T) {
	d := zsh.Diagnostics()
	for _, b := range []string{"typeset", "declare", "integer"} {
		if d.BuiltinBadName[b] != "not valid in this context: %[2]s" {
			t.Errorf("BuiltinBadName[%q] = %q", b, d.BuiltinBadName[b])
		}
		if d.BuiltinBadNameNumeric[b] != "not an identifier: %[2]s" {
			t.Errorf("BuiltinBadNameNumeric[%q] = %q", b, d.BuiltinBadNameNumeric[b])
		}
	}
	if got := zsh.Semantics().TypesetTakesASubscript; got != interp.Yes {
		t.Errorf("TypesetTakesASubscript = %v, want Yes", got)
	}
}
