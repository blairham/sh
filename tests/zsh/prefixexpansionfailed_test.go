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

// A here-document body that will not expand is **not** the prefix's failure,
// and a clean prefix in front of the command must not move it. It has a rule
// of its own — see Semantics.HeredocBodyFailureIsTheRedirections, which in
// this column makes it this shell's own failed expansion and so fatal — so
// the answer has to be the same with and without the prefix, as it is in zsh
// 5.9.2 and in every other column.
//
// This column is where it is visible: the two constructs set the same flag,
// and a prefix door that read it would give the shell up over a prefix that
// expanded perfectly well — or, once the body's own door is in place, would
// grade the body's failure by the prefix's rule.
func TestAFailureThatIsNotThePrefixsIsUnmovedByOne(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const body = " : <<END\n$((1/0))\nEND\necho \"after st=$?\"\n"
	bare, bareSt := runZsh(t, dir, body)
	with, withSt := runZsh(t, dir, "a=ok"+body)
	if bare != with || bareSt != withSt {
		t.Errorf("%q at %d with no prefix, %q at %d with one: "+
			"a clean prefix must not move a failure that is not its own",
			bare, bareSt, with, withSt)
	}
	// The control: the body really did fail and really did cost this shell
	// the rest of the script, so the agreement above is not two runs that
	// did nothing. Measured 2026-09-26 on zsh 5.9.2 under `-f`, where every
	// command this shell runs itself ends over such a body (#4684).
	if !strings.Contains(bare, "division by zero") || strings.Contains(bare, "after") || bareSt != 1 {
		t.Errorf("= %q at %d, want the failure reported and the shell ended at 1", bare, bareSt)
	}
	// And the other half of the pair, which is what says the prefix is not
	// what stopped it: the same prefix in front of a body that expands runs
	// the command, with the value in place.
	if out, st := runZsh(t, dir, "f() { echo \"[$a]\"; }\na=ok f <<END\nok\nEND\n"); out != "[ok]\n" || st != 0 {
		t.Errorf("= %q (status %d), want [ok] at 0", out, st)
	}
}
