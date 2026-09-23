// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestEachPipelineElementFiresTheDebugTrapAtItsOwnLine — a `DEBUG` trap fires
// once per simple element of a pipeline in this dialect, and each firing is at
// **that element's** line.
//
// The firing is the pipeline's rather than the element's own dispatch, so the
// line record still sat on whatever ran before the pipeline: every element
// reported the previous statement's line. Measured 2026-09-23 against bash
// 5.3.15 in the pinned debian:sid-slim, a trap printing `$LINENO`:
//
//	echo a            on line 2, `echo b | cat | cat` on line 3   2 3 3 3
//	the same three elements written on lines 3, 4 and 5           2 3 4 5
//
// Both answered `2 2 2 2` here. `$LINENO` is what a script reads, and it is
// the same record every diagnostic from inside the element is located by
// (#4155).
func TestEachPipelineElementFiresTheDebugTrapAtItsOwnLine(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"three elements on one line",
			"set -T\ntrap 'echo \"@$LINENO\"' DEBUG\n: a\n: b | : | :\n",
			"@3\n@4\n@4\n@4\n",
		},
		{
			"and on three lines of their own",
			"set -T\ntrap 'echo \"@$LINENO\"' DEBUG\n: a\n: b |\n  : |\n  :\n",
			"@3\n@4\n@5\n@6\n",
		},
		// The control: a single command is not a pipeline and fires once, at
		// its own line, which it always did.
		{
			"a lone command is unchanged",
			"set -T\ntrap 'echo \"@$LINENO\"' DEBUG\n: a\n: b\n",
			"@3\n@4\n",
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
