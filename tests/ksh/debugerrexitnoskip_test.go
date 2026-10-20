// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A DEBUG action that turns `errexit` on here **changes the option** and
// skips nothing. zsh reads the same arming as "do not run this command" and
// spends the option saying it; this shell does not, and
// Semantics.DebugActionArmingErrExitSkipsTheCommand is that split.
//
// The row is here rather than beside zsh's because **this is where it can
// fire**. A mutant that ignores the axis and always skips survives the whole
// zsh suite — zsh answers Yes anyway — and this row goes from
// `three four status=0` to no output at all.
//
// It is also here rather than in `dialect/bash` because the bash shapes I
// tried could not arm the option observably: `set -e` in a bash DEBUG body
// leaves `shopt -qo errexit` reading *off* afterwards in the reference and
// here alike, so a bash row asserting "nothing was skipped" passes whether
// or not anything was armed. That row was written, seen to be vacuous
// against this same mutant, and deleted.
//
// Measured 2026-09-30 on ksh93u+ 2012-08-01.
func TestADebugActionArmingErrExitSkipsNothingHere(t *testing.T) {
	const src = "f() { trap 'set -e' DEBUG; echo three; echo four; }\nf; echo \"status=$?\"\n"
	out, st := runKshWithPrelude(t, src)
	const want = "three\nfour\nstatus=0\n"
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q — the arming must change the option, not skip", out, st, want)
	}
}
