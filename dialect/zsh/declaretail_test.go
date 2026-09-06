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

// A bare `set` lists the variables as assignments — quoted the way this shell
// quotes them, and with no functions after them, which is where it parts from
// bash's listing rather than from dash's. Asserted on whole rendered lines,
// through a filter, because the rest of the listing is the machine's.
func TestBareSetListsAssignments(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`v1=plain; v2='has space'; v3="quo'te"; a=(x 'y z'); myfn() { echo hi; }; set`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	for _, want := range []string{
		"v1=plain\n",
		"v2='has space'\n",
		`v3='quo'\''te'` + "\n",
		"a=( x 'y z' )\n",
		// The NUL this shell keeps in IFS is written, not emitted raw: a
		// listing with a NUL in it is binary, and `grep` says so of it.
		`IFS=$' \t\n\C-@'` + "\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bare set: %q missing from %q", want, out)
		}
	}
	if strings.Contains(out, "myfn") {
		t.Errorf("bare set: got %q, want no functions in this shell's listing", out)
	}
}

// A bare `local` lists every parameter with its attributes in *words* — the
// order measured against zsh 5.9.2, one name per combination.
func TestBareLocalListsEveryParameterWithItsAttributes(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { local x=1; local -r r=2; local -i i=3; local -a c=(x "y z"); `+
			`local -A d=([k]="v w"); local -x e=4; local -ir ir=5; local -xr q=1; local u; local; }; f`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	for _, want := range []string{
		"local x=1\n",
		"local readonly r=2\n",
		"integer local i=3\n",
		"array local c=( x 'y z' )\n",
		"association local d=( [k]='v w' )\n",
		"local exported e=4\n",
		"integer local readonly ir=5\n",
		"local readonly exported q=1\n",
		"local u=''\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bare local: %q missing from %q", want, out)
		}
	}
	// A global the function never made local carries no `local` word.
	if !strings.Contains(out, "\nPATH=") && !strings.HasPrefix(out, "PATH=") {
		t.Errorf("bare local: got %q, want the globals listed with no local word", out)
	}
}

// The bare `export` and `readonly` drop the command word, which their own
// `-p` does not — and this shell's `readonly -p` is not even `readonly`.
func TestBareExportAndReadonlyAreAssignmentsAlone(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`export V='a b'; readonly R=2; export; readonly; export -p; readonly -p`)
	want := "V='a b'\nR=2\nexport V='a b'\ntypeset -r R=2\n"
	if st != 0 || out != want {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
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
