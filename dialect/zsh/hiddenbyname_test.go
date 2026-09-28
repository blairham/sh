// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// What a hidden name's value does in a listing, and which listings ask.
//
// `hideval` withholds a value from a listing that **walks** the table and not
// from one that was asked for the name. Every `want` is the reference's own
// answer, measured 2026-09-28 on `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — run `-f` from a script file under `env -i
// PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, one shell per row.

// TestAHiddenNameAskedForByNameWritesItsValue is the row #5000 filed.
//
// **The last three rows are the control**, and they are what make this one row
// rather than a policy: two forms that already agreed here keep agreeing, and
// `-p` is *by name too* and still withholds — so "asked for the name" is not
// the whole of the rule, and a change that lifted the withholding everywhere
// would have moved all three.
func TestAHiddenNameAskedForByNameWritesItsValue(t *testing.T) {
	const decl = "typeset -H h=hv\n"
	for _, tc := range []struct{ name, src, want string }{
		{"asked by name", "typeset h", "h=hv\n"},
		{"asked by name, with a bare sign", "typeset + h", "h=hv\n"},
		// The controls.
		{"the -m form, which already agreed", "typeset -m h", "h=hv\n"},
		{"the -p form, which is by name too", "typeset -p h", "typeset h\n"},
		{"the whole-table walk", `typeset | while IFS= read -r l; do
			case $l in (h*) print -r -- "[$l]";; esac; done`, "[h]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), decl+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// And a **produced** parameter, which is the half of the issue that says the
// answer is not "never for a producer": the reference writes the row, values
// and all, and this shell wrote nothing at all.
//
// The hazard #5000 was filed against is measured not to be one. `typeset
// mapfile` really does write every file in the working directory there and
// `typeset langinfo` a fifty-five key table — so there was nothing to hold
// back, only a listing this shell was not reaching.
//
// Each row refers to its name first, because a produced parameter nothing has
// asked for is in the state #4923 models and both shells write a different
// row for it.
func TestAProducedParameterAskedForByNameWritesItsValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an empty produced array",
			"zmodload zsh/parameter\n: ${#funcstack}\ntypeset funcstack",
			"funcstack=(  )\n",
		},
		{
			// A table with something in it, written by the script so that
			// the row does not depend on which aliases a shell starts with:
			// the reference's own `zsh -f` carries two and this suite's
			// runner carries none, which is a difference about startup and
			// not about this listing.
			"a produced table with entries",
			"zmodload zsh/parameter\nalias zz=ec\n: ${#aliases}\ntypeset aliases",
			"aliases=( [zz]=ec )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
	// A clock, which is the row that says the value really is read at the
	// moment of the listing: the number is not asserted, only that one is
	// there and the name is not written bare.
	t.Run("a produced integer", func(t *testing.T) {
		out, st := runZsh(t, t.TempDir(),
			"zmodload zsh/datetime\n: ${EPOCHSECONDS}\ntypeset EPOCHSECONDS")
		if st != 0 || !strings.HasPrefix(out, "EPOCHSECONDS=1") {
			t.Errorf("= %q (status %d), want a clock behind the `=`", out, st)
		}
	})
	// The control for the walk is the hidden-name row in the test above,
	// which asks the same question of a name a script hid: the walk writes
	// `h` and this listing writes `h=hv`. A produced name's walk row is
	// #4923's and is asserted there.
}
