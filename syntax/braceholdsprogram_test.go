// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// `${x}` and `${ cmd;}` are the same two opening characters and are not the
// same construct, and one dialect blames the two at different lines when the
// input runs out inside them. Nothing in the error could say which form it
// was, because the opener is the same — see Error.HoldsProgram.
//
// Measured 2026-09-12 on bash 5.3.15 over a two-line file, `echo after` being
// the second line:
//
//	echo ${ echo hi   line 3 — the line after the input's last
//	echo ${x          line 1 — the line the `${` is on
func TestAnUnmatchedBraceSaysWhetherItHeldAProgram(t *testing.T) {
	d := Core()
	d.CurrentShellSubstitution = true
	for _, tc := range []struct {
		src     string
		program bool
	}{
		{"echo ${ echo hi\necho after\n", true},
		{"echo ${x\necho after\n", false},
		// The comment rule reaches the same branch: a `#` inside the
		// command form runs to the newline and can carry the closing brace
		// off with it.
		{"echo ${ echo hi # cmt }\necho after\n", true},
		// And an operator that looks like one does not, in the parameter
		// form, where `#` strips a prefix.
		{"echo ${x#a\necho after\n", false},
	} {
		_, err := Parse(tc.src, d)
		var se *Error
		if !errors.As(err, &se) || se.Kind != ErrUnmatched {
			t.Errorf("%q: got %v, want an ErrUnmatched", tc.src, err)
			continue
		}
		if se.HoldsProgram != tc.program {
			t.Errorf("%q: HoldsProgram = %v, want %v", tc.src, se.HoldsProgram, tc.program)
		}
	}
}

// Without the grammar for the command form there is no form to tell apart, so
// the flag is never set and the two spellings are one construct again.
func TestABraceHoldsNoProgramWhereTheGrammarHasNoSuchForm(t *testing.T) {
	for _, src := range []string{"echo ${ echo hi\n", "echo ${x\n"} {
		_, err := Parse(src, Core())
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%q: got %v, want a *Error", src, err)
		}
		if se.HoldsProgram {
			t.Errorf("%q: HoldsProgram set with no such form in the grammar", src)
		}
	}
}
