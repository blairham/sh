// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `history -s` and `history -p` with nothing after them.
//
// Measured 2026-09-21 against bash 5.3.20 at /opt/homebrew/bin/bash — the
// panel's bash, not /bin/bash, which is 3.2 — under `env -i` with a scratch
// HOME and HISTFILE=/dev/null, from a script file with `set -o history`
// written so the reader fills the list:
//
//	history -s a      the list holds `a`, and the builtin's own line is
//	                  gone from it
//	history -s        nothing is stored, 0, no diagnostic — and the
//	                  builtin's own line is **still in the list**
//	history -s ''     an empty entry is stored, and the own line goes
//	history -s '   '  the spaces are stored
//	history -s --     as no operands: nothing stored, own line kept
//	history -p        nothing printed, own line kept
//	history -p ''     an empty line printed, own line gone
//
// So it is the **operand count** and not emptiness, and the two are
// distinguishable: `history -s` stored an empty entry here where bash stores
// none. The own line is the discriminating half — a rule written inside the
// store would have dropped it on the way to doing nothing (#4068).
func TestStoringAndPrintingWithNoOperandsDoNothing(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		// The operand count decides, and the empty operand is on the other
		// side of it: one entry, empty, and the numbering says it is there.
		{"no operands store nothing", "history -s\nhistory\n", ""},
		{"an empty operand stores an entry", "history -s ''\nhistory\n", "    1  \n"},
		{"`--` is no operands", "history -s --\nhistory\n", ""},
		{"spaces are an operand", "history -s '   '\nhistory\n", "    1     \n"},
		// And the store is still a store when it is given something, so
		// the rule above is a threshold rather than a road switched off.
		{"an operand is stored", "history -s a\nhistory\n", "    1  a\n"},
		// `-p` reads the same operands and answers the same shape.
		{"printing nothing prints nothing", "history -p\n", ""},
		{"printing an empty operand writes a line", "history -p ''\n", "\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("out = %q status = %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// And the builtin's own line survives a call that stored nothing, which is
// the half a rule written inside the store would have got wrong.
//
// The list has to hold that line for the question to exist at all, so the
// entries are planted through the same seam a front end reading a script
// fills: `history -s` twice, the second of which is the call under test.
func TestStoringNothingLeavesTheBuiltinsOwnLineAlone(t *testing.T) {
	t.Parallel()
	// `history -s keep` drops no own line — nothing recorded one — and
	// leaves the list holding `keep`. A second call with no operands must
	// leave that list exactly as it is rather than shortening it.
	out, st := runBash(t, t.TempDir(), "history -s keep\nhistory -s\nhistory\n")
	if want := "    1  keep\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	// And the same call with an operand does take a line: two entries in,
	// and the list holds both.
	out, st = runBash(t, t.TempDir(), "history -s keep\nhistory -s second\nhistory\n")
	if want := "    1  keep\n    2  second\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}

// Nothing is said either way, which is worth its own assertion because a
// no-op that complains is a different answer from a no-op.
func TestStoringNothingIsSilent(t *testing.T) {
	t.Parallel()
	out, st := runBash(t, t.TempDir(), "history -s\nhistory -p\necho done\n")
	if want := "done\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	if strings.Contains(out, "history:") {
		t.Errorf("out = %q, want no diagnostic", out)
	}
}
