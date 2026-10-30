// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"syscall"
	"testing"
)

// A signal with no name runs no EXIT trap when another process sends it, as
// none runs when the script sends it itself: the name is what the trap is
// asked about, and a real-time number has none. Measured 2026-10-29 in
// debian:bookworm-slim@sha256:99c0b25c46d0214d5a2091092e7908907dcaecd6a962cde0ac700acee222b6f3,
// GNU bash 5.2.15, `trap 'echo "bye $?"' EXIT` and `kill -40` from another
// process at an external `sleep`, a `read` and a busy loop: killed by it at
// 168, nothing written — where TERM in the same places writes `bye`. See
// interp.Runner.runExitTrap for the self-sent half (#3777).
func TestASignalWithNoNameFromOutsideRunsNoExitTrap(t *testing.T) {
	dieRunningAsAShell()
	ws, out := signaledInAGroupOfItsOwn(t, "^TestASignalWithNoNameFromOutsideRunsNoExitTrap$",
		outsideSignal{
			setup: `trap 'echo "bye $?"' EXIT; `, hold: "(exit 7); " + waitsHere, sig: syscall.Signal(40),
			ownReady: true, env: []string{dyingExitTrapRuns + "=yes", dyingHangupExits + "=no"},
		})
	diedOf(t, ws, out, syscall.Signal(40))
}
