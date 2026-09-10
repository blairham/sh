// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Storing the positional parameters quoted and reading them back is a fixed
// point in this dialect, however many times it is done.
//
// This is the shape the plugin manager on this machine writes once per item in
// a `for` list: `"${(j: :)${(q)@[2,-1]}}"` to hand the rest of the list to an
// extension, `"${(@Q)${(@z)…}}"` and `set --` to take it back. The substrate's
// test names the construct; this one names the shell, because the round trip
// only exists where the flag group, the shell-word split and the unquoting
// flag all do.
//
// Every row runs the trip three times, which is what makes it a measurement
// rather than a coincidence: one added level of backslashes and one removed
// level look alike after a single trip, and only the third shows which way it
// went. Reading `${(q)@[…]}` used to write its result back into the parameters
// themselves, so the count doubled per trip (#1622).
//
// `seen` is the second read, and it is not spare. A `set --` from the trip's
// own result overwrites the parameters whatever the read did to them, so a
// loop that reads once per trip cannot see a write-back at all — it passes
// with the bug in place. The manager reads more than once per item for a real
// reason: it hands the rest of the list to every registered extension, and an
// extension that declines hands nothing back for the `set --` to undo. That
// is the read this stands for, and it is what makes these rows grade the
// thing they are named after.
//
// Measured on zsh 5.9.2, 2026-09-09.
func TestQuotingThePositionalParametersRoundTrips(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"three trips over the whole list change nothing",
			`set -- 'a b' 'c d'
			for i in 1 2 3; do
			  seen="${(j: :)${(q)@[1,-1]}}"
			  packed="${(j: :)${(q)@[1,-1]}}"
			  set -- "${(@Q)${(@z)packed}}"
			done
			printf "[%s]" "$@"`,
			`[a b][c d]`,
		},
		{
			"nor over the list past the first parameter",
			`set -- x 'a b' 'c d'
			for i in 1 2 3; do
			  seen="${(j: :)${(q)@[2,-1]}}"
			  packed="${(j: :)${(q)@[2,-1]}}"
			  set -- head "${(@Q)${(@z)packed}}"
			done
			printf "[%s]" "$@"`,
			`[head][a b][c d]`,
		},
		// The read on its own, so a reader can see which half of the trip was
		// wrong: the quoted text is right and the parameter behind it is what
		// changed.
		{
			"and the read leaves the parameter as it was",
			`set -- 'a b'; printf "[%s]" "${(q)@[1,-1]}"; printf "[%s]" "$1"`,
			`[a\ b][a b]`,
		},
		{
			"read three times running it says the same thing",
			`set -- 'a b'; printf "[%s]" "${(q)@[1,-1]}" "${(q)@[1,-1]}" "${(q)@[1,-1]}"`,
			`[a\ b][a\ b][a\ b]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
