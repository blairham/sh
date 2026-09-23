// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAFailedPipelineFiresTheErrTrapAtItsOwnLine — the ERR trap fires for a
// pipeline that failed, and `$LINENO` in its body is the **pipeline's** line.
//
// It is the ERR half of what #4332 fixed for `DEBUG`, and it has the same
// cause: the elements run in copies of the runner, so nothing moved the
// shell's own line record off whatever ran before the pipeline, and the trap —
// which fires from the shell, after the statement — read that.
//
// Measured 2026-09-23 against bash 5.3.20, a trap printing `$LINENO` with
// `echo 2` on line 4 and a failing pipeline on line 5: `5` there and `4` here.
// The traced prefix said the same thing twice over, `++[5]` against `++[4]`,
// which is what made it two lines of `trap.tests` rather than one (#4157).
func TestAFailedPipelineFiresTheErrTrapAtItsOwnLine(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"three failing elements on one line",
			"trap 'echo \"@$LINENO\"' ERR\n: a\nfalse | false | false\n",
			"@3\n",
		},
		{
			// Written across lines, where the pipeline's own line is its
			// first: bash locates the statement and not the element that
			// failed.
			"and across three lines of their own",
			"trap 'echo \"@$LINENO\"' ERR\n: a\nfalse |\n  false |\n  false\n",
			"@3\n",
		},
		{
			// pipefail makes an earlier element decide the status, and the
			// line is still the pipeline's.
			"with pipefail, where an earlier element decides",
			"set -o pipefail\ntrap 'echo \"@$LINENO\"' ERR\n: a\nfalse | true | true\n",
			"@4\n",
		},
		// The control: a lone failing command is not a pipeline, and its line
		// was never wrong.
		{
			"a lone command is unchanged",
			"trap 'echo \"@$LINENO\"' ERR\n: a\nfalse\n",
			"@3\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashSplit(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want nothing", errs)
			}
		})
	}
}
