// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The lines of `eval`'s text continue from the line the `eval` word is written
// on, and `$LINENO` moves with them (#2462).
//
// Measured 2026-09-12, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a
// script file, bash 5.3.15. **The obvious probe cannot decide this**: with the
// `eval` spread over several physical lines, "the physical line the failing
// text sits on" and "the caller's line plus the text's, less one" are the same
// number. So every case here holds the whole `eval` on one physical line, with
// the newline carried in through a parameter:
//
//	nl='\n'                            (a quoted newline: lines 1 and 2)
//	eval "echo x${nl}echo L=$LINENO"   the eval word is on line 3
//
// The read is the text's line 2, so the three candidate answers are 2 (the
// text's own), 3 (the physical line) and 4 (continued). Real bash says 4.
const oneLineEval = "nl='\n'\n"

func TestEvalTextContinuesTheCallersLines(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "$LINENO inside the text",
			src:  oneLineEval + `eval "echo x${nl}echo L=\$LINENO"` + "\n",
			want: "x\nL=4\n",
		},
		{
			// The offset is the text's and does not outlive it.
			name: "and the line after it is the file's own",
			src:  oneLineEval + `eval "echo x"` + "\necho after=$LINENO\n",
			want: "x\nafter=4\n",
		},
		{
			// A function called from inside the text is not part of it.
			name: "a function called from the text keeps its own lines",
			src:  "f() { echo \"in f L=$LINENO\"; }\n" + oneLineEval + `eval "f"` + "\n",
			want: "in f L=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A run-time failure inside the text is located by the same rule, which is
// what says this is the shell's idea of where it is rather than a wording.
// Measured the same day, with the whole `eval` on line 2 of the file and the
// failing line the text's third: `./b2.sh: line 4: NOPE: unbound variable`.
func TestAFailureInsideEvalTextIsLocatedByTheContinuedLine(t *testing.T) {
	dir := t.TempDir()
	src := "nl='\n'\nset -u\n" + `eval "echo x${nl}echo y${nl}echo \$NOPE"` + "\n"
	out, _ := runBash(t, dir, src)
	if want := "line 6: NOPE: unbound variable"; !strings.Contains(out, want) {
		t.Errorf("got %q, want a line naming %q — the eval is on line 4 and the failure is the text's line 3", out, want)
	}
}
