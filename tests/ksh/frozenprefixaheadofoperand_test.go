// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A frozen entry in a declaration's assignment prefix is refused where the
// walk in front of the operands reaches it, so no operand is expanded, stored
// or traced. Measured 2026-11-02 against ksh93u+ 2012-08-01 (`/bin/ksh`) from
// `-c` and a script file under `env -i PATH=/usr/bin:/bin` with a scratch
// HOME. This expanded every operand first, writing `s` and, under `set -x`,
// the operand's line, and refused the entry behind them all (#7094).
//
// The two rows with a substitution in the prefix say the walk is the early
// check's: the frozen entry's own value is expanded before the refusal, and an
// entry behind it is not expanded at all. The last row is the control, where
// the frozen entry is an operand rather than a prefix entry, so the entry in
// front of it and the operand ahead of it are both written.
func TestAFrozenPrefixEntryIsRefusedBeforeTheOperandsExpand(t *testing.T) {
	t.Parallel()
	const refusal = "ksh: r: is read only\n"
	for _, c := range []struct{ name, src, want string }{
		{"export", `readonly r=0; r=3 export y=$(echo s >&2)`, refusal},
		{"typeset", `readonly r=0; v=1 r=3 typeset y=$(echo s >&2)`, refusal},
		{"an array-literal operand", `readonly r=0; v=1 r=3 typeset -a a=($(echo s >&2))`, refusal},
		{"two operands", `readonly r=0; r=3 typeset -x y=$(echo s >&2) z=$(echo t >&2)`, refusal},
		{"an appending operand", `readonly r=0; r=3 typeset y+=$(echo s >&2)`, refusal},
		{"through command", `readonly r=0; r=3 command typeset y=$(echo s >&2); echo st=$?`, refusal},
		{"traced", `readonly r=0; set -x; v=1 r=3 typeset y=1`, "+ v=1\n" + refusal},
		{"traced, a substitution in the operand", `readonly r=0; set -x; v=1 r=3 export y=$(echo s >&2)`, "+ v=1\n" + refusal},
		{"the frozen entry's own value", `readonly r=0; r=$(echo R >&2) export y=$(echo s >&2)`, "R\n" + refusal},
		{"an entry behind the frozen one", `readonly r=0; r=3 v=$(echo V >&2) typeset y=$(echo s >&2)`, refusal},
		{
			"in a POSIX-form function",
			`readonly r=0; f() { v=1 r=3 typeset y=$(echo s >&2); echo after; }; f; echo out`,
			refusal,
		},
		{
			"control: a frozen operand",
			`readonly r=0; set -x; v=1 typeset y=1 r=2`,
			"+ v=1\n+ y=1\n" + refusal,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != 1 {
				t.Errorf("got %q status %d, want %q status 1", out, st, c.want)
			}
		})
	}
}

// The same refusal from a script file, which it ends: the line after it is
// never reached.
func TestAFrozenPrefixEntryInFrontOfADeclarationEndsTheScript(t *testing.T) {
	t.Parallel()
	out, st := runKshFile(t, "readonly r=0\nv=1 r=3 typeset y=$(echo s >&2)\necho after\n")
	if want := "p.sh: line 2: r: is read only\n"; out != want || st != 1 {
		t.Errorf("got %q status %d, want %q status 1", out, st, want)
	}
}
