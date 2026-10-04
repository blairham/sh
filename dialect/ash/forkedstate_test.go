// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestALoneTrapKeepsTheListing: a forked body that is nothing but `trap`
// lists the parent's traps, and every other body lists nothing — keyed on the
// word as written, not on the boundary. Measured 2026-10-03 in the pinned
// image under `-c`. See Semantics.ALoneTrapCommandKeepsTrapListing.
func TestALoneTrapKeepsTheListing(t *testing.T) {
	src := `trap 'echo x' USR1; t=trap; f() { trap; }
trap | cat; echo "$(trap)"; { trap; } | cat; x=1 trap 2>/dev/null | cat; echo -
(trap); f | cat; $t | cat; \trap | cat; command trap | cat; { trap; } 2>&1 | cat
echo "$(trap; echo z)"; echo "$(trap | cat)"; trap & wait`
	row := "trap -- 'echo x' USR1\n"
	want := strings.Repeat(row, 4) + "-\nz\n\n"
	if out, _ := run(t, src); out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestALoneJobsKeepsTheTable: the same reading for the job table. Measured
// 2026-10-03 in the pinned image under `-c`. See
// SubshellJobsKeptForALoneJobsCommand.
func TestALoneJobsKeepsTheTable(t *testing.T) {
	dir := t.TempDir()
	src := `cd '` + dir + `' || exit
sleep 1 & j=jobs; f() { jobs -p; }
jobs -p | cat >a; (jobs -p) >b; f | cat >c; { jobs -p; } | cat >d
command jobs -p | cat >e; $j -p | cat >g; { :; jobs -p; } | cat >h
x=1 jobs -p 2>/dev/null | cat >i; echo "$(jobs -p)" >k; echo "$(jobs -p | cat)" >l
kill %1
for v in a b c d e g h i k l; do w=; read -r w <$v; case $w in "$!") echo "$v=pid";; *) echo "$v=[$w]";; esac; done`
	want := "a=pid\nb=[]\nc=[]\nd=pid\ne=[]\ng=[]\nh=[]\ni=pid\nk=pid\nl=[]\n"
	if out, _ := run(t, src); out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestErrtraceStopsAtTheFork: the ERR trap stays in the frame that set it,
// and errtrace carries it into a function but not into a subshell. Measured
// 2026-10-03 in the pinned image under `-c`. See ErrtraceReachesSubshells.
func TestErrtraceStopsAtTheFork(t *testing.T) {
	src := `trap 'echo E' ERR; f() { false; echo in-f; }
f; x=$(false; echo hi); ( false; echo sub ); (trap)
set -E; f; x=$(false; echo hi); ( false; echo sub ); (trap); echo end`
	want := "in-f\nsub\nE\nin-f\nsub\nend\n"
	if out, _ := run(t, src); out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestATrapBodyNamesWhereItFired: every line of a trap body reads the line it
// fired on, and the EXIT trap fires on the last command the script ran.
// Measured 2026-10-03 in the pinned image over a script file, which is how
// this helper numbers lines: under `-c` the shell reads one less on every row.
func TestATrapBodyNamesWhereItFired(t *testing.T) {
	src := "trap 'echo a $LINENO\nnosuchcmd 2>/dev/null\necho b $LINENO' USR1\n:\nkill -USR1 $$\n" +
		"trap 'echo at=$LINENO' ERR\n:\nfalse\n" +
		"trap 'echo x $LINENO' EXIT\necho last\n\n# c\n"
	want := "a 5\nb 5\nat=8\nlast\nx 10\n"
	if out, _ := run(t, src); out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}
