// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import "testing"

// When the handler of a trapped signal that arrives during a `read` the
// signal does not end runs, in each dialect — interp.ReadHandlerMoment
// (#7031). Every row is the script below run from -c on 2026-10-31, and the
// wanted output is what the real shell printed: bash 5.3.20 and 3.2.57, zsh
// 5.9.2, ksh93u+ 2012-08-01, dash 0.5.12, and BusyBox ash 1.37.0 in the pinned
// alpine image.
//
// The read is `read -r l <&3` on a fifo opened for reading alone, which is the
// read ksh93 goes on with once a byte of its line is in. `W` is written by the
// background job between the signal and the end of the line, so where `T`
// falls against it says when the handler ran, and `l` in the handler says
// whether the read had finished.

// signalLandsInTheWait has `ab` in the fifo and the read waiting for the rest
// when INT lands, then `W`, then `cd`. The job starts its clock only once the
// shell has written `ab` and is about to read — it says so through q — and the
// second before the signal is a margin for the read to have taken `ab` and be
// waiting, which every column needs it to be: bash runs a handler only for a
// signal that lands in a wait, and dash and ash give the read up there.
const signalLandsInTheWait = `mkfifo p q; exec 4<>p 3<p; l=old; trap 'echo "T l=[$l]"' INT; ` +
	`(read y <q; sleep 1; kill -INT $$; sleep 1; echo W; echo cd >p) & ` +
	`printf ab >p; echo >q; read -r l <&3; echo "st=$? l=[$l]"; wait`

// signalWaitingAsItBegins raises INT from a command substitution in the read's
// own redirection, so it is already waiting as the read begins — nothing about
// that is a race — and the line comes a second later, after `W`. zsh runs the
// handler as that substitution ends rather than at the read; it is the same
// output, and the one this shell, which runs no handler during an expansion,
// can give.
const signalWaitingAsItBegins = `mkfifo p q; exec 4<>p 3<p; l=old; trap 'echo "T l=[$l]"' INT; ` +
	`(read y <q; sleep 1; echo W; echo cd >p) & ` +
	`echo >q; read -r l <&3 5>"$(kill -INT $$)/dev/null"; echo "st=$? l=[$l]"; wait`

func TestAHandlerRunsDuringAReadWhereTheShellsRunIt(t *testing.T) {
	t.Parallel()
	const (
		atTheArrival = "T l=[old]\nW\nst=0 l=[abcd]\n"
		afterTheRead = "W\nT l=[cd]\nst=0 l=[cd]\n"
		atTheStart   = "T l=[old]\nW\nst=0 l=[cd]\n"
	)
	for _, r := range []struct{ name, dialect, src, want string }{
		// In a wait: bash, zsh and ksh93 stop the read to run the handler and
		// go on; dash and ash give the read up and run it after, keeping what
		// they keep of `ab` (#7028).
		{"in a wait", "bash", signalLandsInTheWait, atTheArrival},
		{"in a wait", "zsh", signalLandsInTheWait, atTheArrival},
		{"in a wait", "ksh", signalLandsInTheWait, atTheArrival},
		{"in a wait", "dash", signalLandsInTheWait, "T l=[ab]\nst=1 l=[ab]\nW\n"},
		{"in a wait", "ash", signalLandsInTheWait, "T l=[old]\nst=1 l=[old]\nW\n"},
		// Waiting as it begins: zsh and ksh93 run it then; bash waits for a
		// signal to land in a wait, and none does; dash and ash run it after
		// the read, which no signal from before its wait gives up.
		{"waiting as it begins", "bash", signalWaitingAsItBegins, afterTheRead},
		{"waiting as it begins", "zsh", signalWaitingAsItBegins, atTheStart},
		{"waiting as it begins", "ksh", signalWaitingAsItBegins, atTheStart},
		{"waiting as it begins", "dash", signalWaitingAsItBegins, afterTheRead},
		{"waiting as it begins", "ash", signalWaitingAsItBegins, afterTheRead},
	} {
		t.Run(r.dialect+"/"+r.name, func(t *testing.T) {
			t.Parallel()
			if got := runReleasingFifos(t, presets[r.dialect], r.src); got != r.want {
				t.Errorf("got %q, want %q", got, r.want)
			}
		})
	}
}

// What a `read` does when a handler run in the middle of it returns from the
// function the read is in — interp.Semantics.ReadLeftByAHandlersReturnAssignsWhatArrived
// (#7176). Every row is the script run from -c on 2026-10-31, three runs a row
// with one answer, and the wanted output is what the real shell printed: zsh
// 5.9.2, bash 5.3.20 and ksh93u+ 2012-08-01. The read is the one
// signalLandsInTheWait makes, inside f; the handler returns 5.
const (
	aReturnLandsInTheWait = `mkfifo p q; exec 4<>p 3<p; l=old; trap 'echo T; return 5' INT; ` +
		`f() { read -r l <&3; echo in; }; ` +
		`(read y <q; sleep 1; kill -INT $$; sleep 1; echo W; echo cd >p) & ` +
		`printf ab >p; echo >q; f; echo "out st=$? l=[$l]"; read -r m <&3; echo "m=[$m]"; wait`
	// aReturnWithNothingIn is the same with nothing written before the signal.
	aReturnWithNothingIn = `mkfifo p q; exec 4<>p 3<p; l=old; trap 'echo T; return 5' INT; ` +
		`f() { read -r l <&3; echo in; }; ` +
		`(read y <q; sleep 1; kill -INT $$; sleep 1; echo W; echo cd >p) & ` +
		`echo >q; f; echo "out st=$? l=[$l]"; read -r m <&3; echo "m=[$m]"; wait`
)

func TestAHandlersReturnInTheMiddleOfAReadWhereTheShellsDo(t *testing.T) {
	t.Parallel()
	for _, r := range []struct{ name, dialect, src, want string }{
		// zsh ends the read as at the end of its input — `ab` assigned, the
		// read's 1 the function's status — and bash and ksh93 with no name
		// touched and the handler's 5. Either way `cd` is the next reader's.
		{"ab in", "zsh", aReturnLandsInTheWait, "T\nout st=1 l=[ab]\nW\nm=[cd]\n"},
		{"ab in", "bash", aReturnLandsInTheWait, "T\nout st=5 l=[old]\nW\nm=[cd]\n"},
		{"ab in", "ksh", aReturnLandsInTheWait, "T\nout st=5 l=[old]\nW\nm=[cd]\n"},
		// With nothing in, zsh's end of input clears the name. ksh93 gives
		// this read up for the signal (ReadAbandonedWhen) and runs the
		// handler after it, so its row asks nothing of the axis.
		{"nothing in", "zsh", aReturnWithNothingIn, "T\nout st=1 l=[]\nW\nm=[cd]\n"},
		{"nothing in", "bash", aReturnWithNothingIn, "T\nout st=5 l=[old]\nW\nm=[cd]\n"},
		{"nothing in", "ksh", aReturnWithNothingIn, "T\nout st=5 l=[]\nW\nm=[cd]\n"},
	} {
		t.Run(r.dialect+"/"+r.name, func(t *testing.T) {
			t.Parallel()
			if got := runReleasingFifos(t, presets[r.dialect], r.src); got != r.want {
				t.Errorf("got %q, want %q", got, r.want)
			}
		})
	}
}
