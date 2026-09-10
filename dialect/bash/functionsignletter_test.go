// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// The `f` letter's *sign*, which this shell reads unlike the two that have a
// names-only function listing. Measured 2026-09-10 on bash 5.3.15 and 3.2.57
// with a scrubbed environment.
//
//   - `declare -F` names every function as the declaration that would restore
//     it — `declare -f f` — and `declare -F f` names the one operand bare. Two
//     shapes, and the letter carries them rather than the operand count
//     carrying them everywhere.
//   - `declare +f` is **not** a names-only listing at all. It takes the
//     function attribute *off*, which leaves the bare `declare`: every
//     variable this shell has and then every function. That listing is
//     BareDeclarationListing's unanswered row here, so the letter goes on
//     writing bodies and this row is what says that was a decision (#1754).
//
// Asserted because the axis is what keeps zsh's reading out of this shell: a
// change that applied the plus sign everywhere passes every zsh row and
// silently turns this one into a name listing no bash writes.
func TestThePlusSignOnTheFunctionLetterIsNotANameListing(t *testing.T) {
	if got := bash.Semantics().FunctionNamesUnderPlus; got != interp.No {
		t.Errorf("FunctionNamesUnderPlus = %v, want No", got)
	}
	dir := t.TempDir()
	out, st := runBash(t, dir, "f() { echo x; }\ndeclare +f f")
	if st != 0 {
		t.Fatalf("declare +f f: status = %d, out %q", st, out)
	}
	if !strings.HasPrefix(out, "f ()") || !strings.Contains(out, "echo x") {
		t.Errorf("declare +f f = %q, want the body — the plus is not a name listing in this shell", out)
	}
	// The letter this shell really does spell for the names, in both of its
	// shapes. The pair is the discriminator: a listing that read the operand
	// count alone would write the same thing twice.
	for _, c := range []struct{ line, want string }{
		{"declare -F", "declare -f f\n"},
		{"declare -F f", "f\n"},
	} {
		out, st := runBash(t, t.TempDir(), "f() { echo x; }\n"+c.line)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.line, out, st, c.want)
		}
	}
	// And a lone sign is a name here rather than an option word, which is the
	// other half of the same disagreement.
	if got := bash.Semantics().SignAloneIsAnOptionWord; got != interp.No {
		t.Errorf("SignAloneIsAnOptionWord = %v, want No", got)
	}
	out, st = runBash(t, t.TempDir(), "declare -")
	if st == 0 || !strings.Contains(out, "not a valid identifier") {
		t.Errorf("declare - = %q (status %d), want the sign refused as a name", out, st)
	}
}
