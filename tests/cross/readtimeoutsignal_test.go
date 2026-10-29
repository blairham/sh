// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import "testing"

// What a trapped signal does to a `read -t` still waiting for its first byte,
// in each dialect with the letter (#7032). Every row is the script below run
// from -c on 2026-10-29, with its output copied, on zsh 5.9.2, bash 5.3.20
// (bash 3.2.57 printed the same), ksh93u+ 2012-08-01 and BusyBox ash 1.37.0 in
// the pinned alpine image. See interp.Semantics.ReadTimeoutWaitEndedByATrappedSignal.
//
// zsh ends the read as its deadline would, at 1, and the line written after it
// is the next read's; bash reads on through the signal and takes it. ksh93 and
// ash give the read up for the signal as they give up any read
// (ReadIsAbandonedByATrappedSignal), and are here to show that is unchanged.
// With `ab` in before the signal, zsh, bash and ksh93 all read on to the end
// of the line.
//
// The signal comes a second in and the line a second after it: a margin for a
// loaded runner under the race detector, since a signal that lands before the
// read is waiting is one it reads on over. A shell that reads on where it
// should not prints `l=[late]` long before the ten-second deadline, and one
// that hangs is released by runReleasingFifos. The rows run in parallel with
// each other and with no other test, so that a signal another test raises
// through the kernel cannot reach a Runner here (#7040).
func TestATimedReadWaitingForItsFirstByteIsEndedWhereTheShellsDo(t *testing.T) {
	script := func(first string) string {
		return `mkfifo p; exec 4<>p 3<p; ` + first + `trap "echo T" INT; ` +
			`(sleep 1; kill -INT $$; sleep 1; printf 'late\nend\n' >p) & ` +
			`read -r -t 10 l <&3; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"; wait`
	}
	const ablate = "T\nst=0 l=[ablate]\nnext [end]\n"
	rows := []struct{ name, dialect, src, want string }{
		{"nothing in", "zsh", script(""), "T\nst=1 l=[]\nnext [late]\n"},
		{"nothing in", "bash", script(""), "T\nst=0 l=[late]\nnext [end]\n"},
		{"nothing in", "ksh", script(""), "T\nst=258 l=[]\nnext [late]\n"},
		{"nothing in", "ash", script(""), "T\nst=1 l=[]\nnext [late]\n"},
		{"part in", "zsh", script(`printf ab >p; `), ablate},
		{"part in", "bash", script(`printf ab >p; `), ablate},
		{"part in", "ksh", script(`printf ab >p; `), ablate},
	}
	for _, r := range rows {
		t.Run(r.dialect+"/"+r.name, func(t *testing.T) {
			t.Parallel()
			if got := runReleasingFifos(t, presets[r.dialect], r.src); got != r.want {
				t.Errorf("got %q, want %q", got, r.want)
			}
		})
	}
}
