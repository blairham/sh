// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"strconv"
	"syscall"
	"testing"
)

// A pipeline element keeps the parent's traps as its own in zsh, so a signal
// the element sends its own pid runs the parent's handler in the element and
// the element carries on: interp.Semantics.PipelineElementKeepsTheTraps
// (#7327). Here it died of the signal.
//
// Measured 2026-11-02 under `-c` against zsh 5.9.2, in a process group of its
// own with no terminal; each row's output there is the want below. The noun is
// the element, so its shape does not matter — but `( … )` is a subshell and
// puts the handler back, and a background job is not a pipeline element.
func TestAPipelineElementKeepsTheParentsTraps(t *testing.T) {
	const trap = `trap 'print -r -- T' USR1; `
	died := "out=" + strconv.Itoa(128+int(syscall.SIGUSR1)) + "\n"
	for _, c := range []struct{ src, want string }{
		{trap + `{ kill -USR1 $sysparams[pid]; print -r -- in } | /bin/cat; print -r -- out=$?`, "T\nin\nout=0\n"},
		{
			trap + `f() { kill -USR1 $sysparams[pid]; print -r -- in }; f | /bin/cat; print -r -- out=$?`,
			"T\nin\nout=0\n",
		},
		{
			trap + `: | { kill -USR1 $sysparams[pid]; print -r -- in } | /bin/cat; print -r -- out=$?`,
			"T\nin\nout=0\n",
		},
		{
			`trap 'print -r -- T' TERM; { kill $sysparams[pid]; print -r -- in } | /bin/cat; print -r -- out=$?`,
			"T\nin\nout=0\n",
		},
		{
			`TRAPUSR1() { print -r -- F }; { kill -USR1 $sysparams[pid]; print -r -- in } | /bin/cat; print -r -- out=$?`,
			"F\nin\nout=0\n",
		},
		// The listing is the element's own table: a change to one trap
		// leaves the parent's other one listed.
		{trap + `{ trap '' USR2; trap } | /bin/cat`, "trap -- 'print -r -- T' USR1\ntrap -- '' USR2\n"},
		// And not kept where the element is a subshell, nor in a job.
		{trap + `( kill -USR1 $sysparams[pid]; print -r -- in ) | /bin/cat; print -r -- out=$?`, "out=0\n"},
		{trap + `{ kill -USR1 $sysparams[pid]; print -r -- in } & wait $!; print -r -- out=$?`, died},
	} {
		if got := runZshAnchored(t, c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
