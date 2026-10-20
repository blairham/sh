// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// An empty key written in a table literal is stored here, as any other key
// is. Measured 2026-10-15 against ksh93u+ 2012-08-01, script file and `-c`
// alike, at 0:
//
//	typeset -A a; a=([""]=1 [b]=2); typeset -p a
//	typeset -A a=(['']=1 [b]=2)
//
// This preset left the question unanswered on a note that the literal was
// refused while it was read, so every row below used to write `the shells
// disagree here and no dialect was chosen` and leave the table empty (#6607).
// See interp.Semantics.EmptyKeyInATableLiteral.
func TestAnEmptyKeyInATableLiteralIsStoredHere(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an assignment", `typeset -A a; a=([""]=1 [b]=2); typeset -p a`, "typeset -A a=(['']=1 [b]=2)\n"},
		{"a declaration", `typeset -A a=([""]=1); typeset -p a`, "typeset -A a=(['']=1)\n"},
		{"an expansion that comes to nothing", `typeset -A a; e=; a=([$e]=x [r]=2); typeset -p a`, "typeset -A a=(['']=x [r]=2)\n"},
		{"single quotes", `typeset -A a; a=(['']=x); typeset -p a`, "typeset -A a=(['']=x)\n"},
		{"the append spelling", `typeset -A a; a+=([""]=x); typeset -p a`, "typeset -A a=(['']=x)\n"},
		{"the last of two wins", `typeset -A a; a=([""]=x [""]=y); typeset -p a`, "typeset -A a=(['']=y)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

func TestTheEmptyTableKeyPolicyIsThisDialects(t *testing.T) {
	if got := ksh.Semantics().EmptyKeyInATableLiteral; got != interp.EmptyKeyInATableLiteralAccepted {
		t.Errorf("EmptyKeyInATableLiteral = %v, want accepted", got)
	}
}
