// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"
)

// What a `read` given up for a trapped signal leaves in the stream, per
// dialect that gives it up (#7024). Measured 2026-10-29 against dash 0.5.12,
// ksh93u+ 2012-08-01 and BusyBox ash 1.37.0 in the pinned alpine image, each
// reading a pipe on standard input:
//
//	script: trap "echo T" INT; (sleep 0.3; kill -INT $$) &
//	        read -r l; echo "st=$? l=[$l]"; read -r m; echo "next [$m]"; wait
//
//	nothing until the read has been given up, then `late`:
//	  dash  T  st=1 l=[]     next [late]
//	  ash   T  st=1 l=[]     next [late]
//	  ksh   T  st=258 l=[]   next [late]
//
//	`ab` waiting before the read, `cd` after it has been given up:
//	  dash  T  st=1 l=[ab]   next [cd]
//
// **What arrives after the read was given up is the next reader's, whole.**
// This shell read on a goroutine so a signal could end the wait, and that
// read stayed parked on the stream after the builtin returned: it took the
// next byte to arrive and threw it away, `next [ate]` and `next [d]` in all
// three dialects. A descriptor is now waited on rather than read until its
// byte is there, so a read given up leaves nothing in flight.
//
// The second row is dash's alone because the other two answer it for reasons
// that are not this one: BusyBox ash leaves `l` unassigned where dash keeps
// `ab` (#7028), and ksh93 does not give a read of a descriptor open for
// reading alone up once part of the line is in (`st=0 l=[abcd]`) — a pipe is
// one, and a fifo opened `<>` is not (#7027; TestATrappedSignalGivesUpAReadWhereTheShellsDo).
// What a read given up leaves in the stream is the first row's question, and
// all three answer it alike.
//
// The rest of the input is written once the shell has printed the handler's
// line, which it does only after the read has returned — so nothing about the
// order depends on a clock but the 0.3 s before the signal. `/bin/sleep` by
// path because the harness runs with no PATH of its own.
const readGivenUpTwice = `trap "echo T" INT; (/bin/sleep 0.3; kill -INT $$) & ` +
	`read -r l; echo "st=$? l=[$l]"; read -r m; echo "next [$m]"; wait`

func TestAReadGivenUpForASignalLeavesTheStreamAsTheShellsDo(t *testing.T) {
	for _, row := range []struct {
		dialect, first, rest, want string
	}{
		{"dash", "", "late\n", "T\nst=1 l=[]\nnext [late]\n"},
		{"ash", "", "late\n", "T\nst=1 l=[]\nnext [late]\n"},
		{"ksh", "", "late\n", "T\nst=258 l=[]\nnext [late]\n"},
		{"dash", "ab", "cd\n", "T\nst=1 l=[ab]\nnext [cd]\n"},
	} {
		t.Run(row.dialect+"/"+row.rest[:len(row.rest)-1], func(t *testing.T) {
			got := runOverAPipe(t, presets[row.dialect], readGivenUpTwice, row.first,
				afterTheFirstLine(row.rest))
			if got != row.want {
				t.Errorf("got %q, want %q", got, row.want)
			}
		})
	}
}
