// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A redirection **target** that will not expand, on a command this shell runs
// itself, is the **redirection's** failure here — the only column of the five
// that reads it that way.
//
// So it draws the grid a file that will not open draws: the shell stops for a
// special builtin, which is the POSIX rule this column keeps, and the command
// alone is given up for everything else. Measured 2026-09-26 on ksh93u+
// 2012-08-01 (/bin/ksh — AT&T's own 2012 build, a different lineage from
// ksh93u+m), a target of `$(( 1/0 ))` on its own line and `echo "after st=$?"`
// on the next (#4689).
func TestAFailedRedirectTargetIsTheRedirectionsHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		src  string
		ends bool
	}{
		{`: < $(( 1/0 ))`, true},
		{`read x < $(( 1/0 ))`, false},
		{`echo RAN < $(( 1/0 ))`, false},
		{`f() { echo RAN; }; f < $(( 1/0 ))`, false},
		{`{ echo RAN; } < $(( 1/0 ))`, false},
		{`command : < $(( 1/0 ))`, false},
	} {
		out, st := runKsh(t, dir, c.src+"\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", c.src, out)
		}
		if ended := !strings.Contains(out, "after"); ended != c.ends {
			t.Errorf("%s = %q, want the shell ending=%v", c.src, out, c.ends)
		}
		if c.ends && st != 1 {
			t.Errorf("%s: status = %d, want 1", c.src, st)
		}
		if !c.ends && !strings.Contains(out, "after st=1") {
			t.Errorf("%s = %q, want the contained failure to leave 1", c.src, out)
		}
	}
}

// The pair that holds the noun fixed. "A special builtin" and "a command this
// shell runs itself" agree on every row above except a **function**, a
// **group** and `command :` — all three are commands this shell runs itself,
// and all three carry on here, which is what says the noun is the first one.
func TestTheFailedTargetNounIsASpecialBuiltinHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	special, _ := runKsh(t, dir, ": < $(( 1/0 ))\necho after\n")
	if strings.Contains(special, "after") {
		t.Errorf("a special builtin = %q, want the shell ended", special)
	}
	for _, c := range []struct{ name, src string }{
		{"a function", "f() { echo RAN; }; f < $(( 1/0 ))"},
		{"a group", "{ echo RAN; } < $(( 1/0 ))"},
		{"command :", "command : < $(( 1/0 ))"},
	} {
		out, _ := runKsh(t, dir, c.src+"\necho after\n")
		if !strings.Contains(out, "after") {
			t.Errorf("%s = %q, want this shell carrying on", c.name, out)
		}
	}
}

// The pair that says which of the two readings it is: a **redirection that
// could not be opened** draws that grid row for row here, so the target's
// failure is graded as one rather than as a failed expansion — which is fatal
// in this column whatever it is written on.
func TestAFailedRedirectTargetMatchesAFailedOpenHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`:`, `read x`} {
		open, openSt := runKsh(t, dir, cmd+" < /nonexistent/f\necho \"after st=$?\"\n")
		target, targetSt := runKsh(t, dir, cmd+" < $(( 1/0 ))\necho \"after st=$?\"\n")
		if strings.Contains(open, "after") != strings.Contains(target, "after") || openSt != targetSt {
			t.Errorf("%s: a failed open said %q at %d and a failing target said %q at %d, "+
				"want the same reach", cmd, open, openSt, target, targetSt)
		}
	}
	// The control the comparison needs: the two commands really are graded
	// apart, so the agreement above is a grid rather than one answer twice.
	special, _ := runKsh(t, dir, ": < /nonexistent/f\necho after\n")
	regular, _ := runKsh(t, dir, "read x < /nonexistent/f\necho after\n")
	if strings.Contains(special, "after") || !strings.Contains(regular, "after") {
		t.Errorf("a failed open said %q on a special builtin and %q on a regular one, "+
			"want only the special one fatal", special, regular)
	}
	// And a failed expansion in an ordinary word is fatal here, which is the
	// reading a failed target does **not** take.
	if out, _ := runKsh(t, dir, "echo $(( 1/0 ))\necho after\n"); strings.Contains(out, "after") {
		t.Errorf("= %q, want a failed expansion in a word to end this shell", out)
	}
}

// The contained one is catchable and the fatal one is not, which is the third
// column a script can see.
func TestOnlyTheContainedTargetFailureIsCatchableHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, _ := runKsh(t, dir, "read x < $(( 1/0 )) || echo CAUGHT\n"); !strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a regular builtin's give-up caught by ||", out)
	}
	if out, _ := runKsh(t, dir, ": < $(( 1/0 )) || echo CAUGHT\n"); strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a special builtin's give-up to escape ||", out)
	}
}

// The failure kind is not the noun either. `${q?word}`, a multi-word
// expansion and an unset name under `set -u` all draw the same column as
// `$(( 1/0 ))` — where the same `${q?word}` in an ordinary word ends this
// shell wherever it is written.
func TestTheFailedTargetKindDoesNotMoveTheGridHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, target := range []string{`${q?bad}`, `$many`, `$NOPE`} {
		pre := "unset q; many='a b'; "
		if target == `$NOPE` {
			pre = "set -u; "
		}
		special, _ := runKsh(t, dir, pre+": < "+target+"\necho after\n")
		if strings.Contains(special, "after") {
			t.Errorf("%s on a special builtin = %q, want the shell ended", target, special)
		}
		regular, _ := runKsh(t, dir, pre+"read x < "+target+"\necho \"after st=$?\"\n")
		if !strings.Contains(regular, "after st=1") {
			t.Errorf("%s on a regular builtin = %q, want it carrying on at 1", target, regular)
		}
	}
	if out, _ := runKsh(t, dir, "unset q; echo ${q?bad}\necho after\n"); strings.Contains(out, "after") {
		t.Errorf("= %q, want the same parameter in a word to end this shell", out)
	}
}

// The control the whole file rests on: a target that expands is opened and its
// command runs.
func TestATargetThatExpandsStillOpensHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runKsh(t, dir, "echo RAN < /dev/null\necho \"after st=$?\"\n"); out != "RAN\nafter st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want RAN and after st=0", out, st)
	}
}
