// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// This shell numbers `eval`'s text on from the line the `eval` word is on,
// with bash rather than with dash — measured 2026-09-12 in the pinned alpine
// image, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a script file
// (#2462).
//
// It is the column that had to be *run* rather than reasoned about. There is
// no BusyBox ash on this machine, so a hand probe of the five shells that are
// here answers a different question — and the sibling's answer is not this
// one's: dash reads the text's own line and this reads bash's.
//
//	                                            dash    ash
//	eval on one physical line, text's line 3   line 3  line 4
//	`$LINENO` on the text's line 2             L=2     L=3
//
// **The obvious probe cannot decide it**: with the `eval` spread over several
// physical lines, the physical reading and the continued one give the same
// number. So the sources here keep the whole `eval` on one line, carrying the
// newline in through a parameter, which separates all three answers.
func TestEvalTextContinuesTheCallersLines(t *testing.T) {
	src := "nl='\n'\n" + `eval "echo x${nl}echo L=\$LINENO"` + "\n"
	out, st := runIn(t, src)
	if want := "x\nL=4\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0 — the eval is on line 3 and the read is the text's line 2", out, st, want)
	}
}
