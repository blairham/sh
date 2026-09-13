// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAProducedParameterStaysFrozenBehindAShadow is `local NAME=v` over a
// parameter this shell produces and has frozen.
//
// A local is a fresh binding and the freeze goes with the value, which is why
// `readonly z=1; f(){ local z=5 }` is taken here and in zsh alike. That is the
// answer for a name a *script* froze. For a produced one zsh keeps the freeze,
// and this took the declaration and dropped the value on the floor (#2551).
//
// Row four is the one that decides the shape, and a suite without it would be
// passed by a refusal aimed at the declaration: there the declaration is
// taken, the shadow happens, and the assignment on the *next* line is what
// the shell refuses. Rows five and six are the other two controls — a letter
// is not a value, and an ordinary readonly still thaws.
func TestAProducedParameterStaysFrozenBehindAShadow(t *testing.T) {
	dir := t.TempDir()
	const refused = "f: read-only variable: ARGC\n"
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			name:   "local with a value is refused, fatally",
			src:    `f() { local ARGC=5; print "A=$ARGC"; }; f; print tail`,
			want:   refused,
			status: 1,
		},
		{
			name:   "and so is typeset, and so is an empty value",
			src:    `f() { typeset ARGC=""; print "A=$ARGC"; }; f`,
			want:   refused,
			status: 1,
		},
		{
			name:   "LINENO answers the same way",
			src:    `f() { local LINENO=5; }; f`,
			want:   "f: read-only variable: LINENO\n",
			status: 1,
		},
		{
			name:   "the valueless form is taken and the freeze survives it",
			src:    `f() { local ARGC; print "A=$ARGC"; ARGC=5; print "B=$ARGC"; }; f`,
			want:   "A=0\n" + refused,
			status: 1,
		},
		{
			name:   "a letter is not a value",
			src:    `f() { local -i ARGC; print "A=$ARGC"; }; f; print "out=$ARGC"`,
			want:   "A=0\nout=0\n",
			status: 0,
		},
		{
			name:   "an ordinary readonly is still thawed by the shadow",
			src:    `readonly z=1; f() { local z=5; print "A=$z"; }; f; print "out=$z"`,
			want:   "A=5\nout=1\n",
			status: 0,
		},
		{
			// `-h` is what makes a shadow an ordinary parameter rather than
			// the shell's own, and it lifts the freeze with it. What the
			// name reads back inside the call is the producer's `0` and not
			// the `5` zsh answers, which is the unwired half of the letter
			// recorded in hideModuleParameter — the row is here for the
			// status and the absence of a refusal.
			name:   "the hide letter lifts it",
			src:    `f() { local -h ARGC=5; }; f; print "st=$? out=$ARGC"`,
			want:   "st=0 out=0\n",
			status: 0,
		},
		{
			// The freeze is the outer name's again on return, so a second
			// call is refused exactly as the first was. A scope that put
			// nothing back would pass every row above and fail this one.
			name:   "and the outer name is frozen again on return",
			src:    `f() { local ARGC; }; f; print "one=$?"; ARGC=9`,
			want:   "one=0\nzsh:1: read-only variable: ARGC\n",
			status: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("out = %q (status %d), want %q (status %d)",
					out, st, tc.want, tc.status)
			}
		})
	}
}
