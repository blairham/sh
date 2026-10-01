// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **`sleep` is a builtin here, reading the numbers ksh93u+ reads and
// complaining in its words, with no location** — measured 2026-10-01 on
// ksh93u+ 2012-08-01, which writes these lines byte for byte.
func TestSleepIsABuiltin(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `type sleep
sleep .01; echo a=$?
sleep 0x0.02; echo b=$?
sleep 1e-2; echo c=$?
sleep 0.01s; echo d=$?
sleep 1e; echo e=$?
sleep nan; echo f=$?
sleep +0.01; echo g=$?
sleep -- 0.01; echo h=$?
sleep abc; echo i=$?
sleep 2e-1s; echo j=$?
sleep 0,5; echo k=$?
sleep; echo l=$?
sleep 1 2; echo m=$?
sleep -x 0.01; echo n=$?
sleep -1; echo o=$?`)
	want := "sleep is a shell builtin\na=0\nb=0\nc=0\nd=0\ne=0\nf=0\ng=0\nh=0\n" +
		"sleep: abc: bad number\ni=1\nsleep: 2e-1s: bad number\nj=1\nsleep: 0,5: bad number\nk=1\n" +
		"sleep: one operand expected\nl=1\nsleep: one operand expected\nm=1\n" +
		"sleep: -x: unknown option\nn=0\nsleep: -1: unknown option\nsleep: one operand expected\no=1\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// **A trapped signal ends the sleep**, the handler runs before the next
// command, and the status is the signal's in this shell's encoding — 256 plus
// the number. Measured on ksh93u+: `T` and then 286 for USR1, after the 0.2
// seconds the sender waited rather than the 5 asked for.
func TestATrappedSignalEndsASleep(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `trap 'echo T' USR1
(/bin/sleep 0.2; kill -USR1 $$) &
SECONDS=0; sleep 5; s=$?
(( SECONDS < 4 )) && echo "st=$s quick"`)
	if want := "T\nst=286 quick\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
