// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An assignment whose value would not expand stores nothing: the name keeps
// what it held, an unset name stays unset, and nothing behind it in the list
// is stored.
//
// Measured 2026-10-15, one statement a line so that the line being given up
// and the shell carrying on can be told apart: `x=4` then `x=$((1/0))` leaves
// `[4]` in bash 5.3.20 under `-c`, a file and `-i`, and in zsh 5.9.2, ksh93u+
// and BusyBox ash 1.38 run interactively — the one route where those shells
// reach the next line. `x=$((1/0)) y=6` leaves `y` alone, `y=6 x=$((1/0))`
// keeps the `y=6`, and a value partly expanded before it failed —
// `x=$(echo hi)$((1/0))` — is not stored either.
//
// It stored the empty string, and only without `set -x`: the traced list
// checked for the failure before its store and the untraced one did not, so
// turning tracing on changed the answer (#6544). Each row runs four ways —
// traced and not, and with the plain store's shortcut on and off, since that
// shortcut hands a failed `$(( ))` back to the full road — and all four must
// give the row's answer.
func TestAFailedAssignmentKeepsWhatTheNameHeld(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"the old value is kept", "x=4\nx=$((1/0))\n", "[4][-]"},
		{"an unset name stays unset", "unset x\nx=$((1/0))\n", "[unset][-]"},
		{"nothing behind it is stored", "x=4 y=5\nx=$((1/0)) y=6\n", "[4][5]"},
		{"what is before it is", "x=4 y=5\ny=6 x=$((1/0))\n", "[4][6]"},
		{"a partial value is not stored", "x=4\nx=$(echo hi)$((1/0))\n", "[4][-]"},
		{"a bad expression is not stored", "x=4\nx=$(( } ))\n", "[4][-]"},
	} {
		for _, traced := range []bool{false, true} {
			for _, shortcut := range []bool{false, true} {
				src := c.src
				if traced {
					src = "set -x\n" + src
				}
				src += "echo \"st=$? [${x-unset}][${y--}]\"\n"
				was := SetPlainScalarStoreShortcutForTest(shortcut)
				out, _ := assignFailRun(t, src, Yes, Yes)
				SetPlainScalarStoreShortcutForTest(was)
				if want := "st=1 " + c.want + "\n"; !strings.HasSuffix(out, want) {
					t.Errorf("%s (traced=%v, shortcut=%v): said %q, want it to end %q",
						c.name, traced, shortcut, out, want)
				}
			}
		}
	}
}

// A prefix whose value would not expand is not stored either, which shows
// where the prefix outlives its command: in front of a special builtin, in
// the dialects that keep such a prefix. Measured 2026-10-15 on BusyBox ash
// 1.38 run interactively, `x=4` then `x=$((1/0)) eval :` leaves `x=4`; it
// left the empty string here (#6544).
func TestAFailedPrefixIsNotKeptByASpecialBuiltin(t *testing.T) {
	out, _ := assignFailRun(t, "x=4\nx=$((1/0)) eval :\necho \"[${x-unset}]\"\n", Yes, Yes)
	if !strings.HasSuffix(out, "[4]\n") {
		t.Errorf("said %q, want the old value kept", out)
	}
}

// And `set -x` does not run a command whose prefix would not expand. The
// trace expands the prefix to write it, which put the failure on the record
// before the route that applies the prefix took its own walk; that walk then
// read the failure as one that was already there, and ran the command with
// the name empty. Measured 2026-10-15: dash 0.5.12 and BusyBox ash 1.38 end
// the script at `set -x; x=$((1/0)) eval …` exactly as they do without the
// option (#6544).
func TestATracedPrefixThatWouldNotExpandRunsNothing(t *testing.T) {
	for _, redir := range []string{"", " >/dev/null"} {
		for _, traced := range []bool{false, true} {
			src := "x=4\nx=$((1/0)) eval 'echo RAN[$x]'" + redir + "\necho after\n"
			if traced {
				src = "set -x\n" + src
			}
			out, st := assignFailRun(t, src, No, No)
			if strings.Contains(out, "RAN") || strings.Contains(out, "after") || st != 2 {
				t.Errorf("traced=%v %q: said %q at %d, want the script ended at 2 with nothing run",
					traced, redir, out, st)
			}
		}
	}
}

// The list stops at the failure, so nothing behind it is expanded or stored
// whatever it is: not an element write, which has a store of its own, and not
// a command substitution's side effect. Measured 2026-10-15 on bash 5.3.20:
// `a=(1 2)` then `x=$((1/0)) a[0]=9` leaves `[1 2]`, and `x=$((1/0))
// z=$(echo SIDE >&2)` writes no `SIDE` (#6544).
func TestNothingBehindAFailedAssignmentRuns(t *testing.T) {
	for _, traced := range []bool{false, true} {
		src := "x=$((1/0)) a[0]=9\nx=$((1/0)) z=$(echo SIDE >&2)\necho \"[${a[*]}][${z-unset}]\"\n"
		if traced {
			src = "set -x\n" + src
		}
		// Ahead of the trace, whose line for a literal asks a question this
		// row is not about.
		src = "a=(1 2)\n" + src
		out, _ := assignFailRun(t, src, Yes, Yes)
		if !strings.HasSuffix(out, "[1 2][unset]\n") || strings.Contains(out, "SIDE") {
			t.Errorf("traced=%v: said %q, want nothing behind the failure run", traced, out)
		}
	}
}
