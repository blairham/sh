// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `$LINENO` is read-only here, which no other column in the panel says.
//
// Measured 2026-09-12, zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a scratch
// HOME, over a script file. bash 5.3, bash 3.2, ksh93u+, dash 0.5.12 and
// BusyBox ash all take `unset LINENO` and read the name empty afterwards, and
// all five take `LINENO=9`; this shell refuses both with `read-only variable:
// LINENO` and stops (#2519).
//
// It matters beyond the wording for the reason the ARGC mark matters: a
// produced scalar with no writer takes an assignment into the stored table,
// and a stored value is what a read finds first — so a script could remove or
// overwrite the name this shell is still counting into, and every later
// `$LINENO` would answer from the stored value.
func TestLinenoIsReadonly(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"unset", "print -r -- \"before=$LINENO\"\nunset LINENO\nprint -r -- 'unreached'"},
		{"assignment", "LINENO=9\nprint -r -- 'unreached'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if !strings.Contains(out, "read-only variable: LINENO") || st != 1 {
				t.Errorf("got %q status %d, want the readonly refusal at 1", out, st)
			}
			if strings.Contains(out, "unreached") {
				t.Errorf("got %q, want the refusal to have ended the script", out)
			}
		})
	}
}

// And the mark is no wider than those two, which was measured before it was
// written rather than assumed — a mark that also stopped a function from
// shadowing the name would break a script that does, and nothing about
// `unset` says whether it should.
//
// Measured in the same run: `f() { typeset LINENO; print -r -- $LINENO; }`
// prints 0 in zsh 5.9.2 and the outer count is intact after the call. It is a
// *declaration* with no value; `local LINENO=5` is the assignment and that one
// this shell does refuse.
func TestLinenoStillTakesAValuelessLocalDeclaration(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { typeset LINENO; print -r -- "in=$LINENO"; }
f
print -r -- "out=$LINENO"`)
	if want := "in=0\nout=3\n"; out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0 — the readonly mark must not "+
			"reach a valueless local declaration", out, st, want)
	}
}
