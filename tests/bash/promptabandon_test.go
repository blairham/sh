// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// What an error inside a prompt rendering costs this shell.
//
// **Measured** 2026-09-12 against bash 5.3.15, through `${v@P}` — this
// shell's spelling of the rendering the other one spells `${(%%)v}`. The
// boundary is the same: the failure is reported, the command holding the
// expansion still prints, the command after it runs, and the shell exits 0.
//
// What a *given-up* rendering is worth is not the same, and that is the whole
// reason it is an answer rather than a rule. This shell hands back the text it
// was handed, with the substitutions simply not performed, where the other
// hands back what it drew (#2053).

// TestAGivenUpRenderingKeepsTheTextItWasHanded is that answer, asserted as the
// whole output: a check that only looked for the diagnostic would pass for the
// shell that swallowed the `printf` entirely.
func TestAGivenUpRenderingKeepsTheTextItWasHanded(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, `v='PRE-$((nofunc()))-POST'; printf 'ONE=[%s]\n' "${v@P}"; printf 'TWO=still-running\n'`)
	const want = "bash: line 1: nofunc(): arithmetic syntax error in expression (error token is \"()\")\n" +
		"ONE=[PRE-$((nofunc()))-POST]\n" +
		"TWO=still-running\n"
	if out != want || st != 0 {
		t.Errorf("out = %q, status %d, want %q at 0", out, st, want)
	}
}

// TestAGivenUpRenderingKeepsTheTextEvenWhereASubstitutionSucceeded is what
// says the answer is "the text as it stood" and not "as far as it got": a
// `${V}` that would have expanded comes back unexpanded with the rest.
func TestAGivenUpRenderingKeepsTheTextEvenWhereASubstitutionSucceeded(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, `V=MID; v='PRE-${V}-$((nofunc()))-POST'; printf 'ONE=[%s]\n' "${v@P}"`)
	const want = "bash: line 1: nofunc(): arithmetic syntax error in expression (error token is \"()\")\n" +
		"ONE=[PRE-${V}-$((nofunc()))-POST]\n"
	if out != want || st != 0 {
		t.Errorf("out = %q, status %d, want %q at 0", out, st, want)
	}
}

// TestTheErrorOperatorIsCaughtAtARendering: this shell does not read
// `${x?word}` as a request to stop, so the operand that ends the other shell
// here is an ordinary error and the command still runs. The pair of rows —
// this one and its opposite under dialect/zsh — is what makes
// Semantics.ParamErrorIsAnExitRequest reachable at this boundary.
func TestTheErrorOperatorIsCaughtAtARendering(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, `v='PRE-${NOPEV?gone}-POST'; printf 'ONE=[%s]\n' "${v@P}"; printf 'TWO=still-running\n'`)
	const want = "bash: line 1: NOPEV: gone\n" +
		"ONE=[PRE-${NOPEV?gone}-POST]\n" +
		"TWO=still-running\n"
	if out != want || st != 0 {
		t.Errorf("out = %q, status %d, want %q at 0", out, st, want)
	}
}
