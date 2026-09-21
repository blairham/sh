// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `fc`'s editor road where this shell parts company with the other column
// that has it — an editor that left the file empty.
//
// Measured 2026-09-21 against zsh 5.9.2 under `env -i` with a scratch `HOME`
// and `TMPDIR`, the list seeded with `print -s` (which is what fills it in a
// shell nobody is sitting at), and the editor `cp /dev/null`, which truncates
// what it is handed:
//
//	fc -e trunc; print after        `read error on /tmp/zsh…`, 1, no `after`
//	fc -e trunc || print caught     the same: `||` does not catch it
//	f() { fc -e trunc; print in; }  the same, and `in` never runs
//	( fc -e trunc ); print after    the subshell ends, `after=1` runs
//
// So it is fatal rather than a failing status, and bash's answer to the same
// editor is silence at 0 — which is why it is an axis,
// [interp.Semantics.FcEmptyEditIsAnError], and not a wording. The rest of the
// road is the substrate's and is asserted in interp/fceditor_test.go.

// The complaint names the file, reports 1, and **ends the shell**.
//
// The `print after` is the half a status assertion cannot see: a refusal that
// only set a status would let it run, and the output would hold it.
func TestAnEditorThatEmptiesTheFileEndsTheShell(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runZshOnPath(t, dir,
		"TMPDIR="+dir+"\nprint -s 'echo one'\nfc -e 'cp /dev/null'\nprint after\n")
	if st != 1 {
		t.Errorf("out = %q status = %d, want 1", out, st)
	}
	if !strings.Contains(out, "read error on "+dir+"/") {
		t.Errorf("out = %q, want the refusal naming the file under %q", out, dir)
	}
	if strings.Contains(out, "after") {
		t.Errorf("out = %q, the shell carried on past a refusal that ends it", out)
	}
	// And it is not caught by `||` either, which is what says this is a
	// fatal error rather than a non-zero status a script can read.
	out, st = runZshOnPath(t, dir,
		"TMPDIR="+dir+"\nprint -s 'echo one'\nfc -e 'cp /dev/null' || print caught\n")
	if st != 1 || strings.Contains(out, "caught") {
		t.Errorf("out = %q status = %d, want it uncaught at 1", out, st)
	}
}

// Inside `( )` it ends the subshell alone and the shell around it carries on
// at 1, which is the row that says the give-up stops at a process boundary
// rather than unwinding the whole script from wherever it was raised.
func TestAnEditorThatEmptiesTheFileInASubshellEndsOnlyIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runZshOnPath(t, dir,
		"TMPDIR="+dir+"\nprint -s 'echo one'\n( fc -e 'cp /dev/null' )\nprint \"after=$?\"\n")
	if st != 0 {
		t.Errorf("out = %q status = %d, want 0 — the parent survives", out, st)
	}
	if !strings.HasSuffix(out, "after=1\n") {
		t.Errorf("out = %q, want it to end with the parent reporting 1", out)
	}
}
