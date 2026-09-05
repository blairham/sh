// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "github.com/blairham/sh/interp"

// On darwin the front end does not listen, and the reason is a descriptor
// rather than a signal.
//
// The Go runtime cannot deliver a signal to a channel from a signal handler
// directly — nothing in its note machinery is safe to run there — so on this
// platform it carries the arrival over a **pipe**, opened the first time
// anything asks os/signal for anything. The pipe's ends are ordinary
// descriptors, and they are low ones, because they are opened while the
// process is young.
//
// A shell then places a script's descriptor table over them. `exec 3>f`
// followed by `exec cmd` is a table with 3 in it, and placing 3 is a dup2 onto
// whatever the runtime had put there. Measured: with the shell listening,
// `exec 3>f; exec /bin/sh -c 'echo x >&3'` dies with
//
//	fatal error: signal_recv: inconsistent state
//
// every time, and the replacement never runs. It is the same fault as #695 and
// #731 — a script's numbers written over the runtime's — arriving through
// signal handling rather than through the poller.
//
// And it is **not this change's to fix**, because it is already reachable: a
// script that says `trap 'x' USR1` before it `exec`s takes the same road today,
// since interp asks os/signal for a trapped signal. That is #799. What this
// change would add is that *every* shell asks, so every `exec` with a placed
// descriptor would break rather than only the ones with a trap. A leak of a Go
// stack trace is worse than the dump it replaces if the price is that `exec`
// stops working.
//
// So this platform keeps the dump for now, and the five signals that could be
// intercepted here stay uncaught with the three that could not. Written down
// rather than skipped quietly: driver/die_test.go asserts the behavior on both
// sides, so the day #799 is fixed this file's removal is a test result rather
// than something to remember.
func watchFatalSignals(*interp.Runner) {}
