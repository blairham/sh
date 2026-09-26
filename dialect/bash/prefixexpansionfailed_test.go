// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A command whose assignment prefix could not be expanded does not run, and
// this shell gives up the **line** it was written on and carries on at the
// next one — whatever the command word was. Measured 2026-09-26 on bash
// 5.3.20, each snippet on its own line so that giving up the line can be told
// from ending the shell (#4675).
func TestAPrefixThatWouldNotExpandGivesUpTheLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{
		`a=$((1/0)) echo RAN`,
		`a=$((1/0)) :`,
		`f() { echo RAN; }; a=$((1/0)) f`,
		`a=$((1/0)) /bin/echo RAN`,
		`a=$((1/0)) command /bin/echo RAN`,
	} {
		out, st := runBash(t, dir, src+"; echo SAME\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", src, out)
		}
		if strings.Contains(out, "SAME") {
			t.Errorf("%s = %q, want the rest of the line given up", src, out)
		}
		if !strings.Contains(out, "after st=1") || st != 0 {
			t.Errorf("%s = %q (status %d), want the next line to run and read 1", src, out, st)
		}
	}
}

// And a prefix to a **frozen name** is the other failure and gets the other
// answer here: this shell reports it and runs the command anyway. The pair is
// what says the rule above is keyed on the expansion rather than on the prefix
// not having taken.
func TestAFrozenPrefixStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBash(t, dir, "readonly r=1\nr=2 echo RAN\necho \"after st=$?\"\n")
	if !strings.Contains(out, "readonly") || !strings.Contains(out, "RAN") {
		t.Errorf("= %q, want the refusal reported and the command run anyway", out)
	}
	if !strings.Contains(out, "after st=0") || st != 0 {
		t.Errorf("= %q (status %d), want the command's own success", out, st)
	}
}

// The control: a prefix that expands runs its command with the value in place.
func TestACleanPrefixStillRunsItsCommandHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runBash(t, dir, "f() { echo \"[$a]\"; }\na=ok f\n"); out != "[ok]\n" || st != 0 {
		t.Errorf("= %q (status %d), want [ok] at 0", out, st)
	}
}
