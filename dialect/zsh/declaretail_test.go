// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The declaration long tail, measured against zsh 5.9.2 (2026-09-04).

// zsh keeps a said-back function's opening brace on the header's line,
// indents with tabs, terminates statements with nothing, and gives `then`
// and `do` lines of their own.
func TestTypesetFSaysTheFunctionBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { if true; then echo one; fi; for w in a b; do echo $w; done; }
typeset -f f`)
	want := "f () {\n\tif true\n\tthen\n\t\techo one\n\tfi\n\tfor w in a b\n\tdo\n\t\techo $w\n\tdone\n}\n"
	if out != want || st != 0 {
		t.Errorf("typeset -f f = %q (status %d), want %q", out, st, want)
	}
}

// `-F` is a float's precision here, not bash's function listing — a letter
// this shell has and this engine does not, named as missing.
func TestTypesetCapitalFIsUnimplemented(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `typeset -F v 2>&1; echo st=$?`)
	// The builtin's name is in the location prefix here, as it is for every
	// message this shell writes.
	if !strings.Contains(out, "typeset:1: -F is not implemented yet") || !strings.Contains(out, "st=2") {
		t.Errorf("got %q, want the letter named as missing with status 2", out)
	}
}

// The case attributes fold assignments here too, later ones included.
func TestCaseAttributesFold(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `typeset -u w=abc
echo $w
w=def
echo $w`)
	if out != "ABC\nDEF\n" {
		t.Errorf("got %q, want the attribute folding both assignments", out)
	}
}

// A bare `set` — and a bare `local` — list this shell's whole parameter
// table, tied arrays and special parameters included, which is refused as
// unimplemented rather than approximated.
func TestWholeParameterListingsAreRefused(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `set 2>&1; echo st=$?`)
	if !strings.Contains(out, "not implemented yet") || !strings.Contains(out, "st=2") {
		t.Errorf("bare set: got %q, want the honest refusal", out)
	}
	out, _ = runZsh(t, t.TempDir(), `f() { local x=1; local 2>&1; echo st=$?; }; f`)
	if !strings.Contains(out, "not implemented yet") || !strings.Contains(out, "st=2") {
		t.Errorf("bare local: got %q, want the honest refusal", out)
	}
}

// A bad `local` name: the digit-led complaint, fatal as every declaration's
// bad name is here, with the builtin named by the location machinery.
func TestLocalBadNameIsFatal(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { local 1x=5; echo unreached; }
f
echo after`)
	if !strings.Contains(out, "not an identifier: 1x") {
		t.Errorf("got %q, want the digit-led complaint", out)
	}
	if strings.Contains(out, "after") || st != 1 {
		t.Errorf("got %q (status %d), want the script ended with 1", out, st)
	}
}

// `command -V` says it the way `type` does, shell's name off the line.
func TestCommandCapitalV(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `command -V echo
command -V nosuch; echo st=$?`)
	if !strings.Contains(out, "echo is a shell builtin\n") ||
		!strings.Contains(out, "nosuch not found\n") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the sentence, the bare complaint and status 1", out)
	}
}
