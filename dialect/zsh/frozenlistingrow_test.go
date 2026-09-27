// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// What a `-p` listing writes for `$PPID`, which is nothing.
//
// The name answers the four listing forms exactly as `$ARGC` and `$LINENO` do
// in the reference, and those two have carried ProducedDeclaration.Silent
// since they were implemented — so they already agreed and this one wrote a
// row, twice (#4864). Measured 2026-09-27 on zsh 5.9.2 under `-f` from a
// script file; dialect/zsh/shellownparameters.go has the table.
//
// **The two neighbors are the control and they are in every row below**, run
// in the same shell: a rule that had stopped writing rows for frozen names
// altogether would pass the first two assertions and fail the last two.
func TestTheFrozenProcessIdWritesNoListingRow(t *testing.T) {
	for _, name := range []string{"PPID", "ARGC", "LINENO"} {
		t.Run(name, func(t *testing.T) {
			// `typeset -p NAME` writes nothing, at 0.
			out, st := runZsh(t, t.TempDir(), `typeset -p `+name+`; print -r -- "rc=$?"`)
			if out != "rc=0\n" || st != 0 {
				t.Errorf("typeset -p %s = %q (status %d), want no row at 0", name, out, st)
			}
			// And `readonly -p` writes none either.
			out, _ = runZsh(t, t.TempDir(), `readonly -p`)
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, name+"=") {
					t.Errorf("readonly -p wrote %q, want no row for %s", line, name)
				}
			}
			// The other two forms still write their row, which is what says
			// the name is in the listing and silent to `-p` rather than out
			// of the roster.
			for _, form := range []string{"readonly", "typeset -r"} {
				out, _ = runZsh(t, t.TempDir(), form)
				if !strings.Contains(out, name+"=") {
					t.Errorf("%s = %q, want a row for %s", form, out, name)
				}
			}
		})
	}
}

// And the attributes are untouched: this tells the *listing* the name writes
// nothing, it does not take the freeze or the integer letter off.
func TestTheFrozenProcessIdKeepsItsAttributes(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- ${(t)PPID}`)
	if out != "integer-readonly-special\n" || st != 0 {
		t.Errorf("${(t)PPID} = %q (status %d), want integer-readonly-special", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `PPID=7; print -r -- unreached`)
	if st == 0 || strings.Contains(out, "unreached") {
		t.Errorf("PPID=7 = %q at %d, want a refusal that ends the script", out, st)
	}
	wantWholeLines(t, out, "zsh:1: read-only variable: PPID")
	// A name the *script* freezes still lists, which is the control that
	// separates "this name is silent" from "a frozen name is silent".
	out, st = runZsh(t, t.TempDir(), `typeset -r rx=1; typeset -p rx`)
	if out != "typeset -r rx=1\n" || st != 0 {
		t.Errorf("typeset -p rx = %q (status %d), want the row", out, st)
	}
}
