// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import "testing"

// When the handler of a trapped signal that another process sends while a
// command substitution runs runs, in each dialect —
// interp.Semantics.HandlersRunAsASubstitutionEnds (#7175). Every row is the
// script below run verbatim from -c on 2026-10-31, three runs a row with one
// answer, and the wanted output is what the real shell printed: zsh 5.9.2,
// ksh93u+ 2012-08-01, bash 5.3.20 and 3.2.57, dash 0.5.12, and BusyBox ash
// 1.37.0 in the pinned alpine image.
//
// The signal comes from the script's background job, which is another process
// in every one of them, while the first substitution waits on p; the job then
// lets it go on. The substitution after it writes `E` as it is expanded, so
// `T` before `E` says the handler ran as the one the signal landed in ended.
// That one ends at 3, after an `$(exit 4)` that follows `false`, so `$?` in
// the handler says whose status it was shown: the substitution's in ksh93,
// the one from before it — 4, not the statement's 1 — in zsh, and the
// command's in the columns that wait for it.
const signalLandsInASubstitution = `mkfifo p q; trap 'echo "T st=$?"' INT; ` +
	`(read y <q; kill -INT $$; echo >p) & ` +
	`false; echo Y "$(exit 4)" "$(echo >q; read z <p; exit 3)" "$(echo E >&2)"; echo "after st=$?"; wait`

func TestAHandlerRunsAsASubstitutionEndsWhereTheShellsRunIt(t *testing.T) {
	t.Parallel()
	const afterTheCommand = "E\nY   \nT st=0\nafter st=0\n"
	for _, r := range []struct{ dialect, want string }{
		{"zsh", "T st=4\nE\nY   \nafter st=0\n"},
		{"ksh", "T st=3\nE\nY   \nafter st=0\n"},
		{"bash", afterTheCommand},
		{"dash", afterTheCommand},
		{"ash", afterTheCommand},
	} {
		t.Run(r.dialect, func(t *testing.T) {
			t.Parallel()
			if got := runReleasingFifos(t, presets[r.dialect], signalLandsInASubstitution); got != r.want {
				t.Errorf("got %q, want %q", got, r.want)
			}
		})
	}
}
