// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"fmt"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Two trapped signals waiting together, the first to run resetting the
// second's trap — interp.Semantics.WaitingSignalWithoutItsTrapIsResent
// (#7170). The probe is the issue's own, under `-c`:
//
//	bash 5.3.20, 3.2.57   HUP, two warnings, dies of USR2 (159 on darwin)
//	dash, BusyBox ash     HUP after
//	ksh93u+, zsh 5.9.2    USR2 HUP after — USR2 runs first, so the reset
//	                      comes too late to matter
//
// measured 2026-11-01 (ash in `busybox:latest`). bash's warnings name the
// signal by this platform's number, as bash does: 31 on darwin, 12 on Linux.
const waitingTrapGoneProbe = "trap \"echo HUP; trap - USR2\" HUP; trap \"echo USR2\" USR2\n" +
	"{ kill -USR2 $$; kill -HUP $$; } | /bin/cat; echo after\n"

func TestAWaitingSignalWhoseTrapWasResetDoesWhatEachDialectDoes(t *testing.T) {
	usr2 := int(syscall.SIGUSR2)
	bash := "HUP\n" +
		fmt.Sprintf("bash: line 2: warning: run_pending_traps: bad value in trap_list[%d]: 0x0\n", usr2) +
		fmt.Sprintf("bash: line 2: warning: run_pending_traps: signal handler is SIG_DFL, resending %d (SIGUSR2) to myself\n", usr2)
	for _, tc := range []struct {
		name, want string
		status     int
	}{
		{"bash", bash, 128 + usr2},
		{"dash", "HUP\nafter\n", 0},
		{"ash", "HUP\nafter\n", 0},
		{"ksh", "USR2\nHUP\nafter\n", 0},
		{"zsh", "USR2\nHUP\nafter\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := presets[tc.name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, waitingTrapGoneProbe)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want || st != tc.status {
				t.Errorf("got %q at %d, want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// The order each dialect runs them in decides which handler can reset which,
// so the other two shells reach the question with the signals the other way
// round, and run nothing for the one that lost its trap — zsh where HUP came
// first, ksh93 where USR2's handler resets HUP's. Measured 2026-11-01: `HUP
// after` in zsh 5.9.2 (three runs) and `USR2 after` in ksh93u+ (the signals
// sent a tenth of a second apart during a `/bin/sleep`).
func TestAWaitingSignalWhoseTrapWasResetRunsNothingInZshOrKsh(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"zsh",
			"trap \"echo HUP; trap - USR2\" HUP; trap \"echo USR2\" USR2\n" +
				"{ kill -HUP $$; kill -USR2 $$; } | /bin/cat; echo after\n",
			"HUP\nafter\n",
		},
		{
			"ksh",
			"trap \"echo USR2; trap - HUP\" USR2; trap \"echo HUP\" HUP\n" +
				"{ kill -HUP $$; kill -USR2 $$; } | /bin/cat; echo after\n",
			"USR2\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := presets[tc.name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A signal handler that runs after a pipeline counts its lines from the
// pipeline's, where the two dialects that count them from where the trap
// fired do: measured 2026-11-01 under `-c`, `L=4` in ksh93u+ and zsh 5.9.2.
// The line record used to stay on the statement in front of the pipeline,
// which was `L=2` here, and is what put bash's warnings above a line early.
func TestAHandlerAfterAPipelineFiresOnThePipelinesLine(t *testing.T) {
	src := "trap \"echo L=\\$LINENO\" USR1\n:\n\n{ kill -USR1 $$; } | /bin/cat\necho after\n"
	for _, name := range []string{"ksh", "zsh"} {
		t.Run(name, func(t *testing.T) {
			out, st, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
			if err != nil {
				t.Fatal(err)
			}
			if want := "L=4\nafter\n"; out != want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, want)
			}
		})
	}
}
