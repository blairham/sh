// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A trap body's own commands do not disturb `PIPESTATUS`, any more than they
// disturb `$?`.
//
// The two move together and for one reason: the record is what the command the
// trap fired *at* left behind, and a body is commands of its own. Without this
// a `DEBUG` body that ran anything left its own pipeline's statuses where the
// command's were, and the next `${PIPESTATUS[@]}` read the trap rather than the
// pipeline.
//
// Measured 2026-09-23 on GNU bash 5.3.20 under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` from a script file, with each body running a pipeline of its own so
// that a record which was not kept comes back as the body's:
//
//	trap 'false | false | true' DEBUG; exit 3 | true      [3 0]
//	trap 'false | false' ERR;          exit 3 | exit 5     [3 5]
//	no trap;                           exit 3 | true       [3 0]
//
// Row three is the control: with no body to run there is nothing to disturb it,
// so a test holding only that row could not fail. This shell answered
// `[1 1 0]` and `[1 1]` on the first two.
//
// bash 3.2.57 answers `[1 1 0]` — the reading this shell had — so keeping the
// record across a body is something bash changed between the two rather than a
// rule it always had. 5.3 is the column this dialect follows, as elsewhere.
//
// Found by using a `DEBUG` trap as an instrument to locate a suite row: a trap
// that reports where the shell is must not move the state the next line reads,
// and this one did — which made a whole group of traced differences the
// instrument's own.
func TestATrapBodyDoesNotDisturbThePipelineRecord(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a DEBUG body running a pipeline",
			"trap 'false | false | true' DEBUG\nexit 3 | true\nprintf '[%s]' \"${PIPESTATUS[*]}\"",
			"[3 0]",
		},
		{
			"an ERR body running a pipeline",
			"set -o errtrace\ntrap 'false | false' ERR\nexit 3 | exit 5\nprintf '[%s]' \"${PIPESTATUS[*]}\"",
			"[3 5]",
		},
		{
			"the control: no body to disturb it",
			"exit 3 | true\nprintf '[%s]' \"${PIPESTATUS[*]}\"",
			"[3 0]",
		},
		{
			// And the body still *sees* the record it must not move, which is
			// what makes it usable to report state at all: the third firing is
			// at the last line, and it reads the pipeline's `3 0` — measured
			// identical on bash 5.3.20, the trailing firing of the final
			// command included.
			"a body may read what it must not move",
			"trap 'printf \"<%s>\" \"${PIPESTATUS[*]}\"' DEBUG\nexit 3 | true\nprintf '[%s]' \"${PIPESTATUS[*]}\"",
			"<0><0><3 0>[3 0]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
