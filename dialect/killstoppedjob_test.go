// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// Whether `kill %1` continues a job that is **stopped**, so that the signal
// can land rather than sit pending on a process nothing has run again.
//
// One table rather than a line in each dialect's own file, because the point
// of this axis is that it splits one column off five. A per-dialect assertion
// states each value and none of them states that; and a shell that started
// answering it like zsh would fail here rather than quietly join the
// minority.
//
// Measured 2026-09-25 through a pseudo-terminal — `TERM=dumb`, `PS1`/`PS2`
// exported empty, each shell started interactive — with `sleep 300 &` then
// `kill -STOP %1`, and the process's state read from **outside** the shell by
// `ps -o stat=` on the pid, never out of the job table:
//
//	                     kill -0 %1   kill -WINCH %1   kill -CONT %1
//	zsh 5.9.2            SN           SN               SN
//	bash 5.3.20          T            T                S
//	bash 3.2.57          T            T                S
//	ksh93u+              T            T                S
//	dash 0.5.12          T            T                S
//	BusyBox ash 1.37.0   T            T                S
//
// The ash row is the pinned alpine image, with the state read from
// `/proc/<pid>/stat` because BusyBox's `ps` has neither `-o` nor `-p`.
//
// Two things about how those rows were taken are worth keeping, because
// without either one the table would have come out wrong.
//
// **The fatal signals cannot grade this on a Mac.** A Darwin kernel ends a
// stopped process on a default-fatal signal without waiting to be continued,
// so `kill -TERM %1` is `GONE` in every column here and says nothing at all.
// Linux keeps it pending — the ash row above reads `T` for `kill -TERM %1`
// with the SIGTERM still owed — which is where a shell that does not continue
// loses the signal outright. So the columns that discriminate are the ones
// that *deliver nothing anybody can act on*.
//
// **`kill -CONT %1` is the control, and it is not decoration.** Every column
// moves on it, so the two columns beside it are a difference between the
// shells rather than a probe that could not have seen a continue at all.
func TestWhetherEachDialectContinuesAStoppedJobBeforeSignalingIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		sem  interp.Semantics
		want interp.Answer
	}{
		{"zsh continues it", zsh.Semantics(), interp.Yes},
		{"bash does not", bash.Semantics(), interp.No},
		{"ksh93 does not", ksh.Semantics(), interp.No},
		{"dash does not", dash.Semantics(), interp.No},
		{"ash does not", ash.Semantics(), interp.No},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sem.KillJobSpecContinuesAStoppedJob; got != tc.want {
				t.Errorf("KillJobSpecContinuesAStoppedJob = %v, want %v", got, tc.want)
			}
		})
	}
}
