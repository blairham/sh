// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestKillHasNoOptionComplaint is the ash half of #3139.
//
// **Everything after the dash is a signal to this shell.** There is no option
// letter for `kill` to refuse, so a number out of range, a letter, a word and
// a number with junk behind it are one sentence with the *whole* word in it,
// at status 1. Measured 2026-09-16 against BusyBox 1.37.0 in the pinned
// Alpine image, each under a matching `argv[0]`:
//
//	kill -99 $$    ash: bad signal name '99'      status 1
//	kill -Q $$     ash: bad signal name 'Q'       status 1
//	kill -NOPE $$  ash: bad signal name 'NOPE'    status 1
//	kill -9x $$    ash: bad signal name '9x'      status 1
//	kill -s 99 $$  ash: bad signal name '99'      status 1
//
// This column had dash's answer — `illegal option -9` at 2 — which named a
// letter nobody typed, at a status this shell has no route to for a signal it
// did not recognise. The whole-word rows are what tell the two readings
// apart: `-NOPE` under dash's reading names `-N`.
//
// The wording is asserted here rather than in the suite because it cannot be
// asserted there: BusyBox prints these applet-style, `ash: ...` with no file
// and no line, and the same sentence from this shell names the binary it was
// started as. share/suite/ash/builtins.tests pins the statuses and the
// sentence with its prefix cut off; this pins the sentence itself.
func TestKillHasNoOptionComplaint(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`kill -99 $$`, "bad signal name '99'"},
		{`kill -Q $$`, "bad signal name 'Q'"},
		{`kill -NOPE $$`, "bad signal name 'NOPE'"},
		{`kill -9x $$`, "bad signal name '9x'"},
		{`kill -s 99 $$`, "bad signal name '99'"},
	} {
		out, st := run(t, c.src+`; echo "st=$?"`)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s said %q, want %q in it", c.src, out, c.want)
		}
		if strings.Contains(out, "illegal option") {
			t.Errorf("%s said %q, and this shell has no option complaint for a signal", c.src, out)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s was status %v in %q, want 1", c.src, st, out)
		}
	}
	// The control, so the rows above are known to discriminate: a signal this
	// shell has reaches the target and the complaint, if any, is about the
	// pid rather than the word in front of it.
	if out, _ := run(t, `kill -9 $$ 2>&1 >/dev/null; echo "st=$?"`); strings.Contains(out, "bad signal name") {
		t.Errorf("kill -9 said %q, and 9 is a signal here", out)
	}
}
