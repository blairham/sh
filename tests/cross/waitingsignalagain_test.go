// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A signal that arrives again while it is already waiting, and how often each
// dialect then runs its handler — interp.Semantics'
// WaitingSignalArrivingAgainRunsOnce (#7166).
//
// Measured 2026-11-01 under `-c`, each shell in a process group of its own:
//
//	                       bash 5.3.20, 3.2.57, ksh93u+,
//	                       dash, BusyBox ash 1.38.0       zsh 5.9.2
//	kill -USR1 $$ $$       T after                        T T after
//	kill … $$; kill … $$   T T after                      T T after
//
// The second row is the control: a turn between the two arrivals makes them
// two in every column, so what the first row measures is the waiting and not
// the signal being sent twice. BusyBox ash is in `busybox:latest`. A forked
// body's own arrivals fold the same way, and are pinned beside the parameter
// that reaches them, in tests/bash and tests/zsh.
func TestASignalArrivingAgainWhileItWaitsRunsInEachDialectsWay(t *testing.T) {
	const once, twice = "T\nafter\n", "T\nT\nafter\n"
	for _, row := range []struct {
		name, src string
		want      map[string]string
	}{
		{
			"one kill, twice",
			"trap 'echo T' USR1\nkill -USR1 $$ $$\necho after\n",
			map[string]string{"bash": once, "dash": once, "ash": once, "ksh": once, "zsh": twice},
		},
		{
			"a turn between",
			"trap 'echo T' USR1\nkill -USR1 $$\nkill -USR1 $$\necho after\n",
			map[string]string{"bash": twice, "dash": twice, "ash": twice, "ksh": twice, "zsh": twice},
		},
	} {
		for name, want := range row.want {
			t.Run(row.name+"/"+name, func(t *testing.T) {
				out, st, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, row.src)
				if err != nil {
					t.Fatal(err)
				}
				if out != want || st != 0 {
					t.Errorf("got %q at %d, want %q at 0", out, st, want)
				}
			})
		}
	}
}
