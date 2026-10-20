// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// Nothing is taken back when a coprocess ends here, which is the third answer
// to the question bash and ksh93 answer the other two ways.
//
// Measured 2026-09-12 on zsh 5.9.2: the coprocess is reaped and a `read -p`
// still answers the line it left in the pipe (#2411).
func TestReadingAReapedCoprocessStillAnswers(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`coproc (print hi); wait; read -p a; print "1=$? a=[$a]"`)
	if want := "1=0 a=[hi]\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// And a read that finds nothing left forgets the whole coprocess — the write
// end with the read one, and with no reaping needed: this snippet has no
// `wait` in it and both letters refuse afterwards.
func TestAReadFindingEndOfFileForgetsTheCoprocess(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`coproc (print hi); read -p a; read -p b; print "2=$?"; read -p c; print "3=$?"; print -p x; print "w=$?"`)
	if want := "2=1\nzsh:read:1: -p: no coprocess\n3=1\nzsh:print:1: -p: no coprocess\nw=1\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}
