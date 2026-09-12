// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// What an error inside a prompt rendering costs this shell.
//
// **Measured** 2026-09-12 against zsh 5.9.2, with `setopt promptsubst` and a
// math function nobody registered — the shape powerlevel10k's `PROMPT` opens
// with, `${$((_p9k_on_expand()))+}`, before the theme's own `functions -M` has
// run. The rendering is given up and nothing else is: the diagnostic is
// written, the command holding the expansion still produces its own output,
// the command after it runs, and the shell exits 0.
//
// Every assertion here is the **whole** output. Checking only that the
// diagnostic appeared cannot see this bug at all — the diagnostic was already
// byte-identical with zsh's on the day the rest of the script stopped running
// (#2053).

// TestAFailedRenderingCostsTheRenderingAndNotTheScript is the reported shape,
// unchanged: the value the expansion fails on is the first thing in the
// string, so what the rendering is worth is nothing, and the `print` still
// prints its brackets.
func TestAFailedRenderingCostsTheRenderingAndNotTheScript(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, "setopt promptsubst\ns='${$((nofunc()))+}X'\nprint -r -- \"ONE=[${(%%)s}]\"\nprint -r -- \"TWO=still-running\"\n")
	const want = "zsh:3: unknown function: nofunc\n" +
		"ONE=[]\n" +
		"TWO=still-running\n"
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0: the rendering was given up, not the shell", st)
	}
}

// TestAGivenUpRenderingIsWorthWhatItDrew is the half the row above cannot see,
// because its value begins with the substitution that fails: what is kept is
// the text in front of it rather than nothing at all.
func TestAGivenUpRenderingIsWorthWhatItDrew(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `setopt promptsubst; s='PRE-$((nofunc()))-POST'; print -r -- "ONE=[${(%%)s}]"; print -r -- "TWO=still-running"`)
	const want = "zsh:1: unknown function: nofunc\n" +
		"ONE=[PRE-]\n" +
		"TWO=still-running\n"
	if out != want || st != 0 {
		t.Errorf("out = %q, status %d, want %q at 0", out, st, want)
	}
}

// TestAGivenUpRenderingDropsASubstitutionThatHadSucceeded: the rule is about
// the *first* substitution and not about the failing one. `${V}` expands and
// is thrown away with the rest, so an implementation that simply stopped its
// walk where the failure was would answer `PRE-MID-` and pass the row above.
func TestAGivenUpRenderingDropsASubstitutionThatHadSucceeded(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `setopt promptsubst; V=MID; s='PRE-${V}-$((nofunc()))-POST'; print -r -- "ONE=[${(%%)s}]"`)
	const want = "zsh:1: unknown function: nofunc\n" +
		"ONE=[PRE-]\n"
	if out != want || st != 0 {
		t.Errorf("out = %q, status %d, want %q at 0", out, st, want)
	}
}

// TestTheEscapeTableStillReadsWhatAGivenUpRenderingDrew: the two passes are
// not both given up, so the doubled percent left standing by the abandoned
// expansion is still drawn as one.
func TestTheEscapeTableStillReadsWhatAGivenUpRenderingDrew(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `setopt promptsubst; s='PRE-%%-$((nofunc()))-POST'; print -r -- "ONE=[${(%%)s}]"`)
	const want = "zsh:1: unknown function: nofunc\n" +
		"ONE=[PRE-%-]\n"
	if out != want || st != 0 {
		t.Errorf("out = %q, status %d, want %q at 0", out, st, want)
	}
}

// TestTheErrorOperatorEndsTheShellAtARendering is the half that keeps this
// boundary from catching everything. `${x?word}` is documented by this shell
// as exiting rather than complaining, and it does so here exactly as it does
// inside a file `.` read — so the `print` never runs.
func TestTheErrorOperatorEndsTheShellAtARendering(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `setopt promptsubst; s='PRE-${NOPEV?gone}-POST'; print -r -- "ONE=[${(%%)s}]"; print -r -- "TWO=still-running"`)
	const want = "zsh:1: NOPEV: gone\n"
	if out != want {
		t.Errorf("out = %q, want %q: the operand this shell calls a request to stop is not caught here", out, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestABadSubstitutionInARenderingStillLetsTheCommandRun is the other kind of
// failure a rendering can hold: one that reports itself *and* leaves the
// expansion marked failed. The command holding the word is refused for that
// mark, so a boundary that cleared only the unwinding would catch the error
// and still swallow the `print`.
func TestABadSubstitutionInARenderingStillLetsTheCommandRun(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `setopt promptsubst; s='PRE-${x@ZZ}-POST'; print -r -- "ONE=[${(%%)s}]"; print -r -- "TWO=still-running"`)
	const want = "zsh:1: bad substitution\n" +
		"ONE=[PRE-]\n" +
		"TWO=still-running\n"
	if out != want || st != 0 {
		t.Errorf("out = %q, status %d, want %q at 0", out, st, want)
	}
}

// TestPrintPIsTheSameBoundary: the builtin spelling of the same rendering.
// It is one expansion rather than two by construction, and this is what says
// so from the outside — a boundary written at the `${(%%)…}` reader alone
// would leave this route abandoning the script.
//
// The diagnostic is not asserted here: this shell still names the builtin in
// the location where zsh names only the shell, which is #2131 and is a
// question about the whole panel rather than about this boundary.
func TestPrintPIsTheSameBoundary(t *testing.T) {
	dir := t.TempDir()
	out, st, errs := runZshSplit(t, dir, `setopt promptsubst; print -P 'PRE-$((nofunc()))-POST'; print -r -- "TWO=still-running"`)
	const want = "PRE-\nTWO=still-running\n"
	if out != want || st != 0 {
		t.Errorf("stdout = %q, status %d, want %q at 0", out, st, want)
	}
	if !strings.Contains(errs, "unknown function: nofunc") {
		t.Errorf("stderr = %q, want the failure named", errs)
	}
}
