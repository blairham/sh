// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The `%` parameter flag written twice is a second question, and PROMPT_SUBST
// is what it asks. Measured on zsh 5.9.2, 2026-09-11, with `V=WORLD` and
// `s='a${V}b'`:
//
//	setopt promptsubst     ${(%)s}  a${V}b   ${(%%)s}  aWORLDb
//	unsetopt promptsubst   ${(%)s}  a${V}b   ${(%%)s}  a${V}b
//
// One `%` draws the escapes and never expands; two also run the value through
// parameter, command and arithmetic expansion, gated on the option.
//
// It was found on powerlevel10k, which does not only draw its prompt but
// *measures* it — the theme is a layout engine binary-searching on rendered
// widths, and every one of those goes through `${(%%)…}`. With the values
// unexpanded it was measuring the width of a `${…}` nest rather than of the
// characters a person sees (#2048).
func TestTheDoubledPercentFlagExpandsUnderPromptSubst(t *testing.T) {
	dir := t.TempDir()
	const body = `V=WORLD; s='a${V}b'; print -r -- "[${(%)s}] [${(%%)s}]"`

	if out, st := runZsh(t, dir, "setopt promptsubst; "+body); st != 0 || out != "[a${V}b] [aWORLDb]\n" {
		t.Errorf("with promptsubst: out = %q, status %d, want %q", out, st, "[a${V}b] [aWORLDb]\n")
	}
	if out, st := runZsh(t, dir, body); st != 0 || out != "[a${V}b] [a${V}b]\n" {
		t.Errorf("without promptsubst: out = %q, status %d, want %q", out, st, "[a${V}b] [a${V}b]\n")
	}
}

// Command and arithmetic substitution travel with it — the option names all
// three and the measurement took all three.
func TestTheDoubledFlagRunsCommandAndArithmeticToo(t *testing.T) {
	dir := t.TempDir()
	src := `s='a$(echo C)-$((1+2))b'; print -r -- "[${(%%)s}]"`
	if out, st := runZsh(t, dir, "setopt promptsubst; "+src); st != 0 || out != "[aC-3b]\n" {
		t.Errorf("out = %q, status %d, want %q", out, st, "[aC-3b]\n")
	}
}

// A third `%` adds nothing over the second, which is what says this is a
// two-state question and not a count.
func TestAThirdPercentAddsNothing(t *testing.T) {
	dir := t.TempDir()
	src := `V=WORLD; s='a${V}b'; print -r -- "[${(%%%)s}]"`
	if out, st := runZsh(t, dir, "setopt promptsubst; "+src); st != 0 || out != "[aWORLDb]\n" {
		t.Errorf("out = %q, status %d, want %q", out, st, "[aWORLDb]\n")
	}
}

// `print -P` expands always, subject to the same option — the builtin route
// asks the doubled flag's question rather than the single one's.
func TestPrintPExpandsUnderPromptSubst(t *testing.T) {
	dir := t.TempDir()
	const src = `V=WORLD; print -P 'a${V}b'`
	if out, st := runZsh(t, dir, "setopt promptsubst; "+src); st != 0 || out != "aWORLDb\n" {
		t.Errorf("with promptsubst: out = %q, status %d, want %q", out, st, "aWORLDb\n")
	}
	if out, st := runZsh(t, dir, src); st != 0 || out != "a${V}b\n" {
		t.Errorf("without promptsubst: out = %q, status %d, want %q", out, st, "a${V}b\n")
	}
}

// The escapes themselves are unaffected in either spelling, with the option
// either way — which is the control that says this change is about the
// expansion pass and nothing else.
func TestThePromptEscapesAreUnchangedByTheOption(t *testing.T) {
	dir := t.TempDir()
	const src = `s='a%F{31}z%f b'; print -r -- "[${(%)s}][${(%%)s}]"`
	on, st1 := runZsh(t, dir, "setopt promptsubst; "+src)
	off, st2 := runZsh(t, dir, src)
	if st1 != 0 || st2 != 0 {
		t.Fatalf("statuses %d and %d", st1, st2)
	}
	if on != off {
		t.Errorf("with the option %q, without it %q — the escapes must not move", on, off)
	}
}
