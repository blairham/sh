// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `$BASH_TRAPSIG`, the number of the condition whose trap is running.
//
// Every row was run in bash 5.3.20 and in this shell before it was written
// down — 2026-09-23, `env -i PATH=/usr/bin:/bin` — and the whole table read
// as the empty string here, which is a different answer from every row of it
// including the unset one.
//
// The parameter is read as `${BASH_TRAPSIG-UNSET}` throughout, because the
// question outside a trap is whether it is *there* and not what it holds: an
// empty string and an unset name are a difference a script can see, and the
// first is what this shell answered everywhere.
//
// **A real signal's own number is not a row here, and the reason is the
// harness rather than the answer.** This Runner is in the test process, so a
// `kill -USR1 $$` would be aimed at the test binary — and a `( : ) &` raises
// no SIGCHLD, because a subshell here is a goroutine and not a child. It was
// measured instead, against both shells under `env -i PATH=/usr/bin:/bin`:
// `trap '…' USR1; kill -USR1 $$` reads 30 in bash 5.3.20 and reads 30 here.
// The rows below are the four conditions that are not signals, which is where
// the numbering is this implementation's own work.
func TestTheTrapSignalNumberIsTheConditionsOwn(t *testing.T) {
	const read = `echo "[${BASH_TRAPSIG-UNSET}]"`
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"EXIT is zero",
			`trap '` + read + `' EXIT`,
			"[0]\n",
		},
		{
			// The three conditions that are not signals follow the host's
			// table, which ends at 31 here. Derived rather than written out:
			// see interp.Runner.TrapSignalNumber.
			"DEBUG is one past the last signal",
			`trap '` + read + `' DEBUG; :`,
			"[32]\n",
		},
		{
			"ERR is the one after that",
			`trap '` + read + `' ERR; false`,
			"[33]\n",
		},
		{
			"and RETURN the one after that",
			`f(){ :; }; set -T; trap '` + read + `' RETURN; f`,
			"[34]\n",
		},
		{
			// Unset outside, which `${x-word}` is the only way to ask.
			"and outside a trap it is not there at all",
			`trap 'true' USR1; ` + read,
			"[UNSET]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}
