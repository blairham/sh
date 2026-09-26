// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A redirection **target** that will not expand, on a command this shell runs
// itself, is **this shell's own failed expansion** — not the redirection's.
//
// A failed expansion is fatal here, so the shell ends, whatever the command
// word was. Measured 2026-09-26 on zsh 5.9.2 under `-f` (#4689).
func TestAFailedRedirectTargetEndsTheShellHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{
		`: < $(( 1/0 ))`,
		`read x < $(( 1/0 ))`,
		`f() { echo RAN; }; f < $(( 1/0 ))`,
		`{ echo RAN; } < $(( 1/0 ))`,
	} {
		out, st := runZsh(t, dir, src+"\necho after\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", src, out)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", src, out)
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want 1", src, st)
		}
	}
}

// The pair that says it is not the redirection's: a file that will **not
// open** carries this shell on at 1 on the same two commands, and `||` catches
// it. So the grid a failed open draws here is not the grid a failed target
// draws, which is exactly the reading ksh93 takes and this column does not.
func TestAFailedOpenIsNotFatalHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`:`, `read x`} {
		out, st := runZsh(t, dir, cmd+" < /nonexistent/f\necho \"after st=$?\"\n")
		if !strings.Contains(out, "after st=1") || st != 0 {
			t.Errorf("%s = %q (status %d), want this shell carrying on at 1", cmd, out, st)
		}
	}
	if out, _ := runZsh(t, dir, ": < /nonexistent/f || echo CAUGHT\n"); !strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a failed open caught by ||", out)
	}
}

// The control: a target that expands is opened and its command runs.
func TestATargetThatExpandsStillOpensHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runZsh(t, dir, "echo RAN < /dev/null\necho \"after st=$?\"\n"); out != "RAN\nafter st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want RAN and after st=0", out, st)
	}
}
