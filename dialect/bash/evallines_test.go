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

// The arrangement the axis above was deliberately not measured with, pinned
// here because it is the one people write and the one #2431 reported: an
// `eval` whose text is spread over several physical lines.
//
// It cannot *decide* the axis — with the text laid out this way, "the
// physical line the failing text sits on" and "the caller's line plus the
// text's, less one" are the same number, which is why the probes above hold
// the whole `eval` on one line. What it can do is show that both readings of
// the number arrive at the arrangement a script actually has, over both
// `$LINENO` and a diagnostic at once.
//
// Measured on bash 5.3.15, 2026-09-12. bash 3.2 answers 5, 6 and line 7 here
// — a third rule again, offsetting from the line the `eval` command *ends*
// on — and nothing in this tree models that column.
func TestAMultiLineEvalIsNumberedTheSameWay(t *testing.T) {
	dir := t.TempDir()
	src := "echo one\necho two\neval 'echo E1=$LINENO\necho E2=$LINENO\nnosuchcmd'\n"
	out, _ := runBash(t, dir, src)
	for _, want := range []string{"E1=3\n", "E2=4\n", "line 5: nosuchcmd"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want it to contain %q", out, want)
		}
	}
}
