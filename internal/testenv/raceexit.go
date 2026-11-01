// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package testenv

import "strings"

// noExitPause is the race detector option that takes its pause at exit away.
const noExitPause = "atexit_sleep_ms=0"

// WithoutRaceExitPause returns env for a re-executed copy of this test binary,
// with the race detector told not to pause before the child exits.
//
// # What the pause costs
//
// A binary built with -race sleeps for a second in the main goroutine before
// it exits, so that a goroutine still running gets a last chance to race and
// be reported: the detector's atexit_sleep_ms, 1000 by default. A test binary
// pays it once at the end of its whole run, which is nothing. A case that runs
// in a child of the binary pays it once per child, which is the whole of the
// case: a descriptor-table case measured 1.09 s in a child and 0.07 s with the
// option set, and an empty child exited in 1.1 s against 0.08 s. Thirty-one of
// interp's cases run that way, and under -race they were half a minute of the
// package's serial tail (#7290).
//
// # What it does not cost
//
// The child runs one case and the testing package checks the detector after
// it, so a race inside the case fails it exactly as before, and a race
// reported before the exit still makes the child exit with the detector's
// status. What goes is the extra second for a goroutine the case left behind
// to race after the case has returned — and the same case is still waited out
// in full where the parent binary ends its own run.
//
// Only under -race, because the option means nothing to a binary without the
// detector, and the environment an uninstrumented child is handed is then
// exactly what it was. The detector reads GORACE as space-separated options
// and a later one wins, so the option is added after anything the caller set;
// an earlier GORACE entry is folded in rather than left beside it, because a
// duplicate name in an environment is read differently by different readers.
func WithoutRaceExitPause(env []string) []string {
	if !RaceDetector {
		return env
	}
	const name = "GORACE="
	out := make([]string, 0, len(env)+1)
	var options []string
	for _, kv := range env {
		if value, ok := strings.CutPrefix(kv, name); ok {
			if value != "" {
				options = append(options, value)
			}
			continue
		}
		out = append(out, kv)
	}
	return append(out, name+strings.Join(append(options, noExitPause), " "))
}
