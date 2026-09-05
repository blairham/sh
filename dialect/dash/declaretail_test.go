// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// The declaration long tail, measured against dash (2026-09-04).

// `local` reads no options here: `local -r x` declares a variable named
// `-r`, which is a bad name, and a declaration's bad name ends the script.
func TestLocalOptionsAreBadNames(t *testing.T) {
	out, st := runDash(t, t.TempDir(), `f() { local -r x=5; echo unreached; }
f
echo after`)
	if !strings.Contains(out, "local: -r: bad variable name") {
		t.Errorf("got %q, want the operand refused as a name", out)
	}
	if strings.Contains(out, "unreached") || strings.Contains(out, "after") || st != 2 {
		t.Errorf("got %q (status %d), want the script ended with 2", out, st)
	}
}

// A `local` name led by a digit is the same refusal with the builtin's name
// left off — `1y: bad variable name` — where `-r` above keeps it. Measured
// from both shapes.
func TestLocalDigitLedNameDropsTheBuiltin(t *testing.T) {
	out, st := runDash(t, t.TempDir(), `f() { local 1y=2; }
f`)
	if !strings.Contains(out, "1y: bad variable name") || strings.Contains(out, "local: 1y") {
		t.Errorf("got %q, want the complaint without the builtin's name", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}

// A bare `local` inside a function writes nothing at all.
func TestBareLocalIsSilent(t *testing.T) {
	out, st := runDash(t, t.TempDir(), `f() { local x=1; local; echo st=$?; }
f`)
	if out != "st=0\n" || st != 0 {
		t.Errorf("got %q (status %d), want silence and 0", out, st)
	}
}

// A bare `set` single-quotes every value, doubling out an embedded quote:
// `'quo'"'"'te'`.
func TestBareSetQuotesEverything(t *testing.T) {
	out, st := runDash(t, t.TempDir(), `v1=plain
v3="quo'te"
set`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	for _, want := range []string{"v1='plain'\n", `v3='quo'"'"'te'` + "\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}

// `command -V` answers with `type`'s words here — the shell's name off the
// line — and a missing name reports a missing command's 127.
func TestCommandCapitalV(t *testing.T) {
	out, _ := runDash(t, t.TempDir(), `command -V echo
command -V nosuch; echo st=$?`)
	if !strings.Contains(out, "echo is a shell builtin\n") ||
		!strings.Contains(out, "nosuch: not found") || !strings.Contains(out, "st=127") {
		t.Errorf("got %q, want the sentence, the bare complaint and 127", out)
	}
}
