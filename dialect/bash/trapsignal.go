// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// `BASH_TRAPSIG`: the number of the condition whose trap is running.
//
// bash 5.3 added it, and what it is for is a body shared between conditions:
// one action bound to several signals can only tell which one fired by reading
// this, since the alternative — a body per signal — is what it exists to
// replace.
//
// **Unset outside a trap**, which is not the same as empty and is what
// `SetDynamicPresence` is registered for. Measured 2026-09-23 on bash 5.3.20
// under `env -i PATH=/usr/bin:/bin`, with `${BASH_TRAPSIG-UNSET}` read from
// each place:
//
//	outside any trap        UNSET
//	trap … EXIT             0
//	trap … USR1, kill -USR1 30
//	trap … DEBUG            32
//	trap … ERR              33
//	set -T; trap … RETURN   34
//
// The last three are the host's signal table extended: it ends at 31 here, and
// the three conditions that are not signals follow it in that order. The core
// derives them from the table rather than from these numbers — see
// interp.Runner.TrapSignalNumber — so a host whose table is longer numbers
// them where that host's bash does.
func registerTrapSignalNumber(r *interp.Runner) {
	r.SetDynamic("BASH_TRAPSIG", func(rr *interp.Runner) string {
		number, _ := rr.TrapSignalNumber()
		return strconv.Itoa(number)
	})
	r.SetDynamicPresence("BASH_TRAPSIG", func(rr *interp.Runner) bool {
		_, running := rr.TrapSignalNumber()
		return running
	})
}
