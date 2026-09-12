// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A bare `(` opening an expansion's pattern operand is refused while reading
// here, and takes the script with it (#1430).
//
// Measured 2026-09-12 against ksh93u+ 2012-08-01 with `v=aXb`.
func TestABareGroupOpeningAPatternOperandIsRefused(t *testing.T) {
	for _, src := range []string{
		`v=aXb; echo "[${v#(a)}]"`,
		`v=aXb; echo "[${v%(b)}]"`,
		`v=aXb; echo "[${v##(a)}]"`,
		`v=aXb; echo "[${v%%(b)}]"`,
		`v=aXb; echo "[${v/(a)/Z}]"`,
		`v=aXb; echo "[${v#((a))}]"`,
	} {
		_, err := syntax.Parse(src+"\n", ksh.Dialect())
		if err == nil {
			t.Errorf("%s parsed, want a refusal", src)
			continue
		}
		want := "syntax error at line 1: `(' unexpected"
		if got := ksh.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%s:\n got %q\nwant %q", src, got, want)
		}
		// And the script goes with it, which is this shell's status for a
		// parse failure.
		if got := ksh.Diagnostics().StatusForParseError(err); got != 3 {
			t.Errorf("%s: status %d, want 3", src, got)
		}
	}
}

// The controls. `@(` is this shell's own spelling of a group and is read; an
// escaped parenthesis is an ordinary character; a group that does not open
// the operand is read as it always was; and a **word** operand takes a
// leading `(` — so it is the pattern's reader that refuses and not the brace.
func TestWhatTheBareGroupRefusalDoesNotReach(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=aXb; echo "[${v#@(a)}]"`, "[Xb]"},
		{`v=aXb; echo "[${v#\(a\)}]"`, "[aXb]"},
		{`v=aXb; echo "[${v#a@(X)}]"`, "[b]"},
		{`unset u; echo "[${u:-(a)}]"`, "[(a)]"},
		{`unset u; echo "[${u-(a)}]"`, "[(a)]"},
		{`unset u; echo "[${u:=(a)}]"`, "[(a)]"},
	} {
		out, st := answersRun(t, tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s: got %q st=%d, want %q", tc.src, out, st, tc.want)
		}
	}
}
