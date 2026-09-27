// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// `$status`, which is `$?` under a name.
//
// The name a zsh script writes where a portable one writes `$?`, and it had
// no parameter here at all: `${+status}` was 0, so `if (( status ))` read an
// empty word and `[[ $status -ne 0 ]]` compared against nothing. That is the
// failure worth naming in #4866's ledger — an absent parameter that a
// conditional quietly takes as empty rather than refusing.
//
// **It answers the four listing forms exactly as `$ARGC`, `$LINENO` and
// `$PPID` do**, which is why this is those three's seam and not a fourth.
// Measured 2026-09-27 on zsh 5.9.2, `-f` from a script file, all four names
// in one run:
//
//	                   status                        ARGC
//	bare typeset       integer 10 readonly status=0  integer 10 readonly ARGC=0
//	bare readonly      status=0                      ARGC=0
//	typeset -r         status=0                      ARGC=0
//	typeset -p NAME    nothing, at 0                 nothing, at 0
//	${(t)NAME}         integer-readonly-special      integer-readonly-special
//	NAME=3             read-only variable, at 1      read-only variable, at 1
//
// so the integer letter, base ten, the freeze and
// [interp.ProducedDeclaration.Silent] are the whole of it. The value is read
// at the moment of the read rather than stored, which is the point: measured
// in the same run, `false; print $status` is 1 and a function that returns 5
// leaves `$status` at 5 — the same number `$?` has, from the same place.
func registerTheLastStatus(r *interp.Runner) {
	r.SetDynamic("status", func(rr *interp.Runner) string {
		return strconv.Itoa(rr.ExitStatus())
	})
	r.MarkReadonly("status")
	r.SetDynamicDeclaration("status", interp.ProducedDeclaration{Integer: true, Base: 10, Silent: true})
}
