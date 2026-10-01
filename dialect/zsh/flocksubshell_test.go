// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// **A lock is the process's, and a subshell is another process.** Measured
// against zsh 5.9.2, where each of these bodies is a fork: the shell asking
// again for what it holds is 0, and a subshell, a command substitution or a
// non-last pipeline element asking for it is refused — while the last element
// of a pipeline is the shell itself and is 0. Two read locks share; a write
// lock excludes a read one. A process substitution is a process too, `-u`
// gives a lock back and so does closing its descriptor, and a job that has to wait for one lets the shell carry
// on — the shell that started it would otherwise never reach the `-u` the job
// is waiting for.
func TestALockExcludesTheShellsOtherProcesses(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, st, errs := runZshSplitWithSystem(t, dir, `zsystem flock f
zsystem flock -t 0 f; print -r -- "again=$?"
( zsystem flock -t 0 f ); print -r -- "subshell=$?"
x=$(zsystem flock -t 0 f; print -r -- $?); print -r -- "subst=$x"
read v < <(zsystem flock -t 0 f 2>/dev/null; print $?); print -r -- "procsubst=$v"
zsystem flock -t 0 f | :; print -r -- "left=$pipestatus"
: | zsystem flock -t 0 f; print -r -- "last=$pipestatus"
zsystem flock -r g 2>/dev/null; : >g; zsystem flock -r -f rd g
( zsystem flock -r -t 0 g ); print -r -- "readread=$?"
( zsystem flock -t 0 g ); print -r -- "readwrite=$?"
: >h; zsystem flock -f fd h; zsystem flock -u $fd
( zsystem flock -t 0 h ); print -r -- "unlocked=$?"
zsystem flock -f fd h; exec {fd}>&-
( zsystem flock -t 0 h ); print -r -- "closed=$?"
zsystem flock -f fd h
( zsystem flock h; print -r -- "job took it" ) &
print -r -- "shell went on"
zsystem flock -u $fd; wait`)
	want := "again=0\nsubshell=1\nsubst=1\nprocsubst=1\nleft=1 0\nlast=0 0\nreadread=0\nreadwrite=1\nunlocked=0\nclosed=0\nshell went on\njob took it\n"
	if out != want || st != 0 {
		t.Errorf("statuses = %q (status %d), want %q", out, st, want)
	}
	wantWholeLines(t, errs,
		"zsh:zsystem:3: failed to lock file f: resource temporarily unavailable",
		"zsh:zsystem:4: failed to lock file f: resource temporarily unavailable",
		"zsh:zsystem:6: failed to lock file f: resource temporarily unavailable",
		"zsh:zsystem:10: failed to lock file g: resource temporarily unavailable",
	)
}

// **A lock a background job holds is held until the job ends, and the shell
// that started the job does not wait for it to start before carrying on.**
// The second half is what makes the first one observable: here a job is a
// goroutine, and the shell used to wait for it to report a process — which a
// body of builtins only does by ending — so the job had already let the lock
// go by the time the next line asked. Measured against zsh 5.9.2: an ask is
// refused while the job holds it, a timed wait runs out silently at 2, and a
// wait with no end returns 0 once the job is over.
func TestABackgroundJobHoldsItsLockUntilItEnds(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, st, errs := runZshSplitWithSystem(t, dir, `zmodload zsh/zselect
( zselect -t 30; print -r -- job ) &
print -r -- shell
wait
( zsystem flock f && : >held && zselect -t 60; : >done ) &
while [[ ! -f held && ! -f done ]]; do zselect -t 1; done
[[ -f done ]] && print -r -- "job ended too soon"
zsystem flock -t 0 f; print -r -- "ask=$?"
zsystem flock -t 0.1 f; print -r -- "timed=$?"
zsystem flock f; print -r -- "wait=$? done=${$(print -r -- done(N)):-no}"
wait`)
	want := "shell\njob\nask=1\ntimed=2\nwait=0 done=done\n"
	if out != want || st != 0 {
		t.Errorf("statuses = %q (status %d), want %q", out, st, want)
	}
	wantWholeLines(t, errs, "zsh:zsystem:8: failed to lock file f: resource temporarily unavailable")
	if got := len(splitLines(errs)); got != 1 {
		t.Errorf("stderr = %q, want exactly one complaint", errs)
	}
}
