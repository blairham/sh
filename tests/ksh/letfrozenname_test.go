// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestAFrozenNameRefusedInsideLetKeepsTheBuiltin is #3568.
//
// This shell writes a builtin's complaint with the bracketed line form and the
// builtin's own name in front. The frozen-name refusal raised inside `let` got
// neither, while the two sentences around it got both.
//
// Measured 2026-09-18, `env -i PATH=/usr/bin:/bin LC_ALL=C ksh x.sh` over
//
//	readonly x=1
//	let x=2
//	let "y=1/0"
//	read x </dev/null
//
//	line          ksh93u+ 2012-08-01                    this shell, before
//	let x=2       x.sh[2]: let: x: is read only         x.sh: line 2: x: is read only
//	let "y=1/0"   x.sh[3]: let: y=1/0: divide by zero   the same
//	read x        x.sh[4]: read: warning: x: is read only   the same
//
// Two things went at once and only on that row — the location form and the
// builtin's name — and the neighbors are the controls: an arithmetic failure
// raised from the same builtin keeps both, and the same refusal raised from
// `read` keeps both.
//
// One table decides both, and it is the one `set` and `read` are already in:
// Diagnostics.ReadonlyRefusalNamesBuiltin. What was missing beside the entry
// is the *form* — the write is one the builtin made, so it reaches the
// sentence that carries a name, where a bare assignment's does not.
//
// `(( x=2 ))` is the discriminator that says this is the builtin's and not the
// arithmetic's: the same write through the arithmetic command is
// `x.sh: line N: x: is read only` there, with no builtin in it, and ends the
// script. The status and the fatality here are #3470's and unchanged — 1, and
// the script runs on.
func TestAFrozenNameRefusedInsideLetKeepsTheBuiltin(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		"readonly x=1\nlet x=2\necho \"A=$?\"\nlet \"y=1/0\"\necho \"B=$?\"")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "let: x: is read only") {
		t.Errorf("said %q, want the builtin named in the sentence", out)
	}
	if !strings.Contains(out, "[2]: let: x") {
		t.Errorf("said %q, want this shell's bracketed line form under it", out)
	}
	if strings.Contains(out, "line 2") {
		t.Errorf("said %q, want no `line N` form — that is the language's location", out)
	}
	// The neighbor is the control: the same builtin's arithmetic failure has
	// carried both all along, and so does the status and the running on.
	if !strings.Contains(out, "[4]: let: y=1/0: divide by zero") {
		t.Errorf("said %q, want the arithmetic failure unmoved", out)
	}
	if !strings.Contains(out, "A=1") || !strings.Contains(out, "B=1") {
		t.Errorf("said %q, want both at 1 with the script running on", out)
	}
	if st != 0 {
		t.Errorf("the script reported %d, want 0 — neither refusal is fatal here", st)
	}
	// And the arithmetic *command* is the discriminator: the same write
	// through `(( ))` carries neither the builtin nor its location.
	out, _, err = preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		"readonly x=1\n(( x=3 ))")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(out, "let") || !strings.Contains(out, "line 2") {
		t.Errorf("the arithmetic command said %q, want no builtin and the language's location", out)
	}
}
