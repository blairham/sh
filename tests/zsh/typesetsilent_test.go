// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A valueless declaration of a name the running scope already holds writes
// that name back — `f(){ local s; s=1; local s; }` says `s=1` — and
// TYPESET_SILENT is how a script asks it not to. Measured on zsh 5.9.2,
// 2026-09-11, in both directions:
//
//	zsh -f -c 'f(){ local s; s=1; local s; }; f'                       s=1
//	zsh -f -c 'setopt typeset_silent; f(){ local s; s=1; local s; }; f'  (nothing)
//
// The option was recorded rather than implemented, so the shell reported it
// on and went on narrating. powerlevel10k sets it in its `emulate -L zsh`
// intro and re-declares `local` names inside loops, which put nine lines on
// stdout before every prompt of a real interactive session (#2033).
func TestTypesetSilentQuietsAValuelessRedeclaration(t *testing.T) {
	// The axis this option is the inverse of: zsh lists, and the option on is
	// that axis answering No.
	if got := zsh.Semantics().ValuelessDeclarationOfAHeldNameListsIt; got != interp.Yes {
		t.Errorf("ValuelessDeclarationOfAHeldNameListsIt = %v, want Yes", got)
	}

	dir := t.TempDir()
	const redeclare = `f(){ local s; s=1; local s; }; f`

	if out, st := runZsh(t, dir, redeclare); st != 0 || out != "s=1\n" {
		t.Errorf("without the option: out = %q, status %d, want %q — the listing itself is zsh's",
			out, st, "s=1\n")
	}
	if out, st := runZsh(t, dir, "setopt typeset_silent; "+redeclare); st != 0 || out != "" {
		t.Errorf("with typeset_silent on: out = %q, status %d, want nothing", out, st)
	}
	// And back: unsetting it restores the listing, so the name is a switch
	// and not a one-way door.
	if out, st := runZsh(t, dir, "setopt typeset_silent; unsetopt typeset_silent; "+redeclare); st != 0 || out != "s=1\n" {
		t.Errorf("after unsetopt: out = %q, status %d, want %q", out, st, "s=1\n")
	}
}

// The state is read off the axis rather than a stored bit, which is what
// keeps a subshell's change inside the subshell — the same bargain `multios`
// strikes. `$(setopt typeset_silent; …)` must not quiet the parent.
func TestTypesetSilentStaysInTheSubshellThatSetIt(t *testing.T) {
	dir := t.TempDir()
	src := `(setopt typeset_silent; f(){ local s; s=1; local s; }; f); g(){ local t; t=2; local t; }; g`
	out, st := runZsh(t, dir, src)
	if st != 0 || out != "t=2\n" {
		t.Errorf("out = %q, status %d, want %q — the subshell is quiet and the parent is not", out, st, "t=2\n")
	}
}

// `setopt` reports what it was asked for, which it did while recorded too —
// the point is that it now reports a name something reads.
func TestTypesetSilentIsStillReportedBothWays(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `setopt typeset_silent; [[ -o typesetsilent ]] && echo on; unsetopt typeset_silent; [[ -o typesetsilent ]] || echo off`)
	if st != 0 || out != "on\noff\n" {
		t.Errorf("out = %q, status %d, want %q", out, st, "on\noff\n")
	}
}
