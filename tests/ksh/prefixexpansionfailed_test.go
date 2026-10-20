// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A command whose assignment prefix could not be expanded does not run, and
// here the failure ends the **shell** for a special builtin and a function and
// is contained in the command for a regular builtin and an external — the same
// split this shell makes for a prefix it refuses, and for the same reason:
// those are the commands it reaches by way of a child.
//
// Measured 2026-09-26 on ksh93u+ 2012-08-01, each snippet on its own line
// (#4675). `command` is transparent here, so what it names decides.
func TestAPrefixThatWouldNotExpandEndsTheShellByCommandKind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		src  string
		ends bool
	}{
		{`a=$((1/0)) echo RAN`, false},
		{`a=$((1/0)) :`, true},
		{`f() { echo RAN; }; a=$((1/0)) f`, true},
		{`a=$((1/0)) /bin/echo RAN`, false},
		{`a=$((1/0)) command /bin/echo RAN`, false},
		// `${q?word}` splits the same way, which is what says the
		// containment is the prefix's and not the arithmetic's.
		{`a=${q?bad} echo RAN`, false},
		{`a=${q?bad} :`, true},
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

// The contained one is catchable and the fatal one is not.
func TestOnlyTheContainedPrefixFailureIsCatchable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, _ := runKsh(t, dir, "a=$((1/0)) echo RAN || echo CAUGHT\n"); !strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a regular builtin's failure caught by ||", out)
	}
	if out, _ := runKsh(t, dir, "a=$((1/0)) : || echo CAUGHT\n"); strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a special builtin's failure to escape ||", out)
	}
}

// The control: a prefix that expands runs its command with the value in place.
func TestACleanPrefixStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runKsh(t, dir, "f() { echo \"[$a]\"; }\na=ok f\n"); out != "[ok]\n" || st != 0 {
		t.Errorf("= %q (status %d), want [ok] at 0", out, st)
	}
}
