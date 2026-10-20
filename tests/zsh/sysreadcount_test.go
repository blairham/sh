// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **`sysread -c` says how a read ended, and a failed `-o` keeps the bytes.**
// Measured against zsh 5.9.2 one line at a time: a read that failed sets the
// count to -1 and an end of input to 0 — `-s 0` is one — while `-t` on a
// descriptor that is not one leaves it as it was. A diversion that cannot be
// written puts the bytes in the parameter the caller named, and leaves a
// defaulted REPLY alone; one that can be written leaves the parameter alone.
func TestSysreadCountsAFailureAndKeepsWhatItCouldNotDivert(t *testing.T) {
	out, st, _ := runZshSplitWithSystem(t, t.TempDir(), `n=old; sysread -t 1 -i 9 -c n; print -r -- "wait=$? $n"
n=old; v=old; sysread -i 9 -c n v; print -r -- "fail=$? $n $v"
n=old; sysread -c n </dev/null; print -r -- "end=$? $n"
n=old; print -n ab | sysread -s 0 -c n; print -r -- "none=$? $n"
v=old; print -n ab | sysread -o 9 -c n v; print -r -- "unwritten=$? $n $v"
REPLY=old; print -n ab | sysread -o 9; print -r -- "default=$? $REPLY"
v=old; print -n hi | sysread -o 1 v; print -r -- " written=$? $v"`)
	want := "wait=2 old\nfail=2 -1 old\nend=5 0\nnone=5 0\nunwritten=3 2 ab\ndefault=3 old\nhi written=0 old\n"
	if out != want || st != 0 {
		t.Errorf("sysread = %q (status %d), want %q", out, st, want)
	}
}
