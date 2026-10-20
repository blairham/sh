// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// The table letter written beside its own operand does *not* reach that
// operand's subscript here: the attribute has not landed when the operand is
// read, so `k` is evaluated as the expression it looks like and the number it
// comes to is the key.
//
// Measured 2026-09-12 on ksh93u+ 2012-08-01:
//
//	typeset -A m[k]=v        typeset -A m=([0]=v)
//	k=7; typeset -A m[k]=v   typeset -A m=([7]=v)
//	typeset -A m[1+1]=v      typeset -A m=([2]=v)
//	typeset -A m; typeset m[k]=v   typeset -A m=([k]=v)
//
// #1380 recorded the first row as the subscript being *discarded* and the
// value landing under `0`. The second and third rows say otherwise and cannot
// agree by accident: `0` is what the unset name `k` evaluates to.
func TestATableLetterDoesNotReachItsOwnOperandsSubscript(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`k=7; typeset -A m[k]=v; typeset -p m; echo "k=[${m[k]}] seven=[${m[7]}]"`)
	if want := "typeset -A m=([7]=v)\nk=[] seven=[v]\n"; out != want || st != 0 {
		t.Errorf("a set name gave %q at %d, want %q at 0", out, st, want)
	}

	out, st = runKsh(t, t.TempDir(), `typeset -A m[1+1]=v; typeset -p m`)
	if want := "typeset -A m=([2]=v)\n"; out != want || st != 0 {
		t.Errorf("an expression gave %q at %d, want %q at 0", out, st, want)
	}

	// The control: a table declared on an earlier command takes the key, so
	// the axis is about the letter and not about tables.
	out, st = runKsh(t, t.TempDir(), `k=7; typeset -A m; typeset m[k]=v; typeset -p m`)
	if want := "typeset -A m=([k]=v)\n"; out != want || st != 0 {
		t.Errorf("a table declared earlier gave %q at %d, want %q at 0", out, st, want)
	}
}
