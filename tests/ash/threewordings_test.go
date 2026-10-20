// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// Three sentences that were still ours rather than this shell's after #2761
// gave every builtin's complaint the right location (#2801).
//
// Measured 2026-09-14, BusyBox v1.37.0 in the pinned alpine image, each line
// alone in a script file under `env -i`. The prefix is identical in every row
// and the assertions below carry it, because the location is what the
// previous change moved and a wording pinned without it would not notice a
// regression there.
func TestThreeWordingsThatWereNotOurs(t *testing.T) {
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"the slot getopts writes into",
			`getopts; echo "st=$?"`,
			": getopts: line 1: usage: getopts optstring var [arg]",
			"`var` where bash and the substrate write `name`. dash says `var` too and " +
				"opens with a capital; bash and ksh93 print their usage line with nothing " +
				"in front of it at all, and zsh writes none and counts the operands instead",
		},
		{
			"an operand wait will not read",
			`wait abc; echo "st=$?"`,
			": wait: line 1: Illegal number: abc",
			"the same sentence this shell writes for `shift -1`, `exit abc`, `return abc`, " +
				"`break abc` and `continue abc` — one reader for all of them, where ours " +
				"had a second one here saying `abc: not a pid`",
		},
		{
			"a division nobody can perform",
			`: $((1/0))`,
			"ash: divide by zero",
			"`divide` where the substrate, dash, both bashes and zsh write `division`. " +
				"No builtin in the location, because an arithmetic failure is the shell's " +
				"own here and not the `:`'s — which is the other half of #2761, and no " +
				"line either, because the shell's own diagnostics carry none on the route " +
				"this helper takes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runIn(t, tc.src); !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q — %s", out, tc.want, tc.why)
			}
		})
	}
}

// `wait` still reaches the job table for a word that *is* a spec, which is
// what says the number reader above was added beside it rather than over it:
// `%1` is `no such job` and `abc` is `Illegal number`, from one builtin.
func TestWaitStillNamesAJobItCannotFind(t *testing.T) {
	out, _ := runIn(t, `wait %1; echo "st=$?"`)
	if want := ": wait: line 1: %1: no such job"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
}
