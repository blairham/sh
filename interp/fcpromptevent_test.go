// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A relative `fc` operand where the shell **has** an event number of its own.
//
// [Semantics.FcRelativeEventNeedsTheShellsOwnEventNumber] counts `-k` back
// from the number the shell's own command has. A script has none, so every
// relative operand lands below the oldest entry and the three magnitudes
// cannot be told apart — which is what the axis's own table measures. A shell
// drawing a prompt does have one: the line a person typed is recorded and
// numbered, so `fc`'s own line has a number and `-k` is that number less k.
//
// Measured 2026-09-30 against zsh 5.9.2 from /opt/homebrew/bin/zsh, `-fis`
// with the lines fed on stdin under `unsetopt PROMPT_SP; PROMPT="";
// exec 2>&1`. Seven events were primed, so the `fc` line's own number is 8:
//
//	fc -l -1        event 7
//	fc -l -2        events 6 and 7
//	fc -l -3        events 5, 6 and 7
//	fc -l -2 -1     events 6 and 7
//	fc -l -1 -1     event 7
//	fc -l -2 -3     events 6 then 5, descending
//	fc -l 5 -1      events 5, 6 and 7
//	fc -l -3 6      events 5 and 6
//	fc -ln -2       the two newest lines, unnumbered
//	fc -l -6        events 2 to 7
//	fc -l -7 -7     event 1 alone
//	fc -l -8 -8     `fc: no such event: 0`
//	fc -l 0         the whole list — the floor, not this count with k of 0
//	fc -l -0        `fc: event not found: -0` — a word, not a count
//
// **The two columns below are the point of the test.** Every row is asserted
// with a prompt and without one, the axis held at the same answer in both, so
// the only thing varying is whether the shell has a number to count from. A
// table with the prompt column alone would pass just as well against a change
// that counted back from the end of the list unconditionally, which is the
// *other* answer to that axis and wrong for a script.
//
// `fc -l -6` is in the table and is deliberately **not** load-bearing: with
// five entries it reaches past the oldest and is clamped, so it reads the
// same as the floor. `-5 -5` is the row that separates them — event 1 alone
// where the floor refuses — and `-6 -6` is the far side, refusing by
// arithmetic rather than by floor.
func TestARelativeOperandCountsBackFromAPromptsOwnEvent(t *testing.T) {
	t.Parallel()
	const whole = "1\t alpha\n2\t beta\n3\t gamma\n4\t delta\n5\t epsilon\n"
	// Five entries numbered from 1, so the `fc` line's own number is 6.
	for _, c := range []struct {
		src string
		// atAPrompt is what the shell answers with an event number of its
		// own; asScript is the same call with none.
		atAPrompt  string
		promptStat int
		asScript   string
		scriptStat int
	}{
		{"fc -l -1", "5\t epsilon\n", 0, whole, 0},
		{"fc -l -2", "4\t delta\n5\t epsilon\n", 0, whole, 0},
		{"fc -l -5", whole, 0, whole, 0},
		// Past the oldest: clamped, and so not a row that tells the two
		// readings apart. Kept because the reference clamps here too.
		{"fc -l -6", whole, 0, whole, 0},
		{"fc -l -2 -1", "4\t delta\n5\t epsilon\n", 0, "sh: fc: no such event: 0\n", 1},
		{"fc -l -1 -1", "5\t epsilon\n", 0, "sh: fc: no such event: 0\n", 1},
		// Descending, because the ends resolved the far way round.
		{"fc -l -2 -3", "4\t delta\n3\t gamma\n", 0, "sh: fc: no such event: 0\n", 1},
		// The row that separates the count from the floor: one entry named
		// twice, where the floor has nothing to name.
		{"fc -l -5 -5", "1\t alpha\n", 0, "sh: fc: no such event: 0\n", 1},
		// And the far side of it: the arithmetic reaches 0 and says so,
		// which is the same words the floor uses and a different reason.
		{"fc -l -6 -6", "sh: fc: no such event: 0\n", 1, "sh: fc: no such event: 0\n", 1},
		// A relative end beside a written one. Without a prompt the `-1`
		// floors below the oldest, the range is brought inside the list and
		// runs the far way round — `2` then `1`, which the reference does
		// too on a script and is not a refusal.
		{"fc -l 2 -1", "2\t beta\n3\t gamma\n4\t delta\n5\t epsilon\n", 0, "2\t beta\n1\t alpha\n", 0},
		// `-n` drops the numbers and nothing else.
		{"fc -ln -2", "\t delta\n\t epsilon\n", 0, "\t alpha\n\t beta\n\t gamma\n\t delta\n\t epsilon\n", 0},
		// `0` is where the count already ended, not this count with k of
		// nought, so it keeps the floor on both routes.
		{"fc -l 0", whole, 0, whole, 0},
		// A positive operand never reaches any of this.
		{"fc -l 4", "4\t delta\n5\t epsilon\n", 0, "4\t delta\n5\t epsilon\n", 0},
	} {
		t.Run(c.src, func(t *testing.T) {
			t.Parallel()
			for _, at := range []struct {
				name   string
				prompt bool
				want   string
				status int
			}{
				{"at a prompt", true, c.atAPrompt, c.promptStat},
				{"as a script", false, c.asScript, c.scriptStat},
			} {
				t.Run(at.name, func(t *testing.T) {
					out, status := run(t, c.src, func(r *Runner) {
						fcFive().install(r)
						sem := *r.Semantics
						sem.FcEventOutOfRangeIsAnError = Yes
						sem.FcRelativeEventNeedsTheShellsOwnEventNumber = Yes
						sem.FcNewestEntryIsTheCurrentLine = No
						r.Semantics = &sem
						r.Interactive = at.prompt
					})
					if out != at.want || status != at.status {
						t.Errorf("%q wrote %q at %d, want %q at %d",
							c.src, out, status, at.want, at.status)
					}
				})
			}
		})
	}
}
