// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A fatal bad name declares every well-formed operand first, wherever it
// stands — measured 2026-09-07 from a script file, reading the names back
// from an EXIT trap because the fatality ends everything written after it.
func TestAFatalBadNameDeclaresTheWholeListFirst(t *testing.T) {
	dir := t.TempDir()
	trap := `trap 'echo "[${ok1-U}][${ok2-U}]"' EXIT; `
	for _, src := range []string{
		`export ":" ok1=1 ok2=2`,
		`export ok1=1 ":" ok2=2`,
		`export ok1=1 ok2=2 ":"`,
		`typeset ok1=1 ":" ok2=2`,
		`readonly ok1=1 ":" ok2=2`,
	} {
		out, st := runZsh(t, dir, trap+src+`; echo NOT-FATAL`)
		// NOT-FATAL is deliberately absent: the refusal still ends the
		// script, and only the trap runs after it.
		if !strings.HasSuffix(out, "[1][2]\n") || strings.Contains(out, "NOT-FATAL") || st != 1 {
			t.Errorf("%s = %q (status %d), want it to end [1][2] at 1", src, out, st)
		}
	}
}

// And `unset` removes them before it stops, which is the same rule read the
// other way round.
func TestAFatalBadNameToUnsetRemovesTheNamesFirst(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`ok1=1 ok2=2; trap 'echo "[${ok1-U}][${ok2-U}]"' EXIT; unset ok1 ":" ok2; echo NOT-FATAL`)
	want := "zsh:unset:1: :: invalid parameter name\n[U][U]\n"
	if out != want || st != 1 {
		t.Errorf("unset ok1 \":\" ok2 = %q (status %d), want %q at 1", out, st, want)
	}
}
