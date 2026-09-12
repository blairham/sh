// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The declaration long tail, measured against ksh93u+ (2026-09-04).

// `typeset -g` does not exist here, and typeset is one of this shell's own
// special builtins, so the refusal — its wording, its usage lines — ends
// the script.
func TestTypesetUnknownOptionIsFatal(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `typeset -g v=1
echo after`)
	if !strings.Contains(out, "typeset: -g: unknown option") {
		t.Errorf("got %q, want the unknown-option complaint", out)
	}
	if !strings.Contains(out, "Usage: typeset [-bflmnprstuxACHS]") ||
		!strings.Contains(out, "Or: typeset [ options ] -f [name...]") {
		t.Errorf("got %q, want the engine's usage lines after it", out)
	}
	if strings.Contains(out, "after") || st != 2 {
		t.Errorf("got %q (status %d), want the script ended with 2", out, st)
	}
}

// `typeset -f` lists the functions here — #1494 built it, and
// dialect/ksh/functionlisting_test.go is where the listing itself is
// asserted. What is left in this file is the letter beside it: `-t` traces a
// function, which this shell does not do, so it is named as missing and the
// failure is fatal the way every typeset failure is here.
func TestTypesetFTracingIsUnimplemented(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `f() { echo hi; }
typeset -ft f
echo after`)
	if !strings.Contains(out, "typeset: -t is not implemented yet") {
		t.Errorf("got %q, want the letter named as missing", out)
	}
	if strings.Contains(out, "after") || st != 2 {
		t.Errorf("got %q (status %d), want the script ended with 2", out, st)
	}
	// The control, and the reason the letter is refused rather than dropped:
	// with it gone the same line is a listing, so a shell that accepted `-t`
	// and ignored it would write the body where this shell writes nothing.
	out, st = runKsh(t, t.TempDir(), `f() { echo hi; }
typeset -f f`)
	if out != "f(){ echo hi; }\n" || st != 0 {
		t.Errorf("got %q (status %d), want the listing", out, st)
	}
}

// The case attributes fold assignments, and `-p` lists them back in this
// engine's shape: one flag word each, case letters between -r and -i.
func TestCaseAttributesFoldAndList(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `typeset -u w=abc
echo $w
typeset -p w`)
	if out != "ABC\ntypeset -u w=ABC\n" {
		t.Errorf("got %q, want the folded value and the listing", out)
	}
}

// A bare `set` lists the variables alone — no functions — bare until a
// value needs quoting and `$'...'` from there.
func TestBareSetListsVariablesAlone(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `v2='has space'
v3="quo'te"
myfn() { echo hi; }
set`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	for _, want := range []string{"v2='has space'\n", `v3=$'quo\'te'` + "\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
	if strings.Contains(out, "myfn") {
		t.Errorf("got %q, want no functions in the listing", out)
	}
}

// `command -V` blames `command` for a name that is nothing, status 1.
func TestCommandCapitalV(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `command -V echo
command -V nosuch; echo st=$?`)
	if !strings.Contains(out, "echo is a shell builtin\n") ||
		!strings.Contains(out, "command: nosuch: not found") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the sentence, the complaint and status 1", out)
	}
}
