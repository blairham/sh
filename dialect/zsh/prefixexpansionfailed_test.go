// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A command whose assignment prefix could not be expanded does not run, and
// here the failure ends the **shell** — except where the command word is an
// external, which this shell reaches by way of a child, so the failure is the
// child's and the script carries on at 1.
//
// Measured 2026-09-26 on zsh 5.9.2 under `-f`, each snippet on its own line
// (#4675). `command echo` is on the external side because `command` is not
// transparent here: it is the word that was written that decides.
func TestAPrefixThatWouldNotExpandEndsTheShell(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		src  string
		ends bool
	}{
		{`a=$((1/0)) echo RAN`, true},
		{`a=$((1/0)) :`, true},
		{`f() { echo RAN; }; a=$((1/0)) f`, true},
		{`a=$((1/0)) /bin/echo RAN`, false},
		{`a=$((1/0)) command /bin/echo RAN`, false},
		// And `${q?word}` is the same three columns of answers, which is
		// what says the containment belongs to the prefix and not to the
		// arithmetic: `echo ${q?bad}` ends this shell wherever it is written.
		{`a=${q?bad} echo RAN`, true},
		{`a=${q?bad} /bin/echo RAN`, false},
	} {
		out, st := runZsh(t, dir, c.src+"\necho \"after st=$?\"\n")
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

// The contained one is catchable, which is the half that says the failure was
// the child's rather than renumbered: the fatal readings escape `||`.
func TestOnlyTheContainedPrefixFailureIsCatchable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, _ := runZsh(t, dir, "a=$((1/0)) /bin/echo RAN || echo CAUGHT\n"); !strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want the external's failure caught by ||", out)
	}
	if out, _ := runZsh(t, dir, "a=$((1/0)) echo RAN || echo CAUGHT\n"); strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a builtin's failure to escape ||", out)
	}
}

// The control: a prefix that expands runs its command with the value in place.
func TestACleanPrefixStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runZsh(t, dir, "f() { echo \"[$a]\"; }\na=ok f\n"); out != "[ok]\n" || st != 0 {
		t.Errorf("= %q (status %d), want [ok] at 0", out, st)
	}
}
