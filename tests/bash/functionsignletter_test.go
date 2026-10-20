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
//     function attribute *off*, and what that leaves depends on whether an
//     operand was written. Measured again 2026-09-12 on bash 5.3.15 with a
//     scrubbed environment: with an operand it is a **silent 0** that leaves
//     the function defined — `declare -F` after it still names it — and with
//     none it is the bare `declare`, which is that shell's `set` listing to
//     the byte (`diff <(declare +f) <(declare)` and `diff <(declare) <(set)`
//     are both empty). This engine wrote the *bodies* for both, because the
//     bare listing was unanswered and the letter had nothing to fall through
//     to (#1754).
//
// Asserted because the axis is what keeps zsh's reading out of this shell: a
// change that applied the plus sign everywhere passes every zsh row and
// silently turns this one into a name listing no bash writes.
func TestThePlusSignOnTheFunctionLetterIsNotANameListing(t *testing.T) {
	if got := bash.Semantics().FunctionNamesUnderPlus; got != interp.No {
		t.Errorf("FunctionNamesUnderPlus = %v, want No", got)
	}
	// With an operand: nothing written, and the function survives — which is
	// what says the letter was read and then had nothing to say, rather than
	// having been ignored on the way to a listing.
	out, st := runBash(t, t.TempDir(), "f() { echo x; }\ndeclare +f f\ndeclare -F")
	if out != "declare -f f\n" || st != 0 {
		t.Errorf("declare +f f = %q (status %d), want silence and the function still defined", out, st)
	}
	// With none: the bare word's listing, which carries variables as well as
	// functions. The `v=1` is what tells it from the function listing.
	//
	// Three spellings of it, because the letter and its sign must all arrive
	// at the same place: `declare +f`, `declare +F` and the bare word.
	for _, line := range []string{"declare +f", "declare +F", "declare"} {
		out, st := runBash(t, t.TempDir(), "v=1\nf() { echo x; }\n"+line)
		if st != 0 || !strings.Contains(out, "v=1\n") || !strings.Contains(out, "f () \n") {
			t.Errorf("%s = %q (status %d), want the bare listing: variables and then functions",
				line, out, st)
		}
	}
	// And that listing is the `set` listing rather than one of its own,
	// which is the whole of what the form says. Asserted on the value, since
	// comparing two runs would compare two `PPID`s as well.
	if got := bash.Semantics().BareTypesetListing; got != interp.BareLocalListsWhatSetLists {
		t.Errorf("BareTypesetListing = %v, want BareLocalListsWhatSetLists", got)
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
