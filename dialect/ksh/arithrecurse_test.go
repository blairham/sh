// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// What this shell does at the end of the chase, measured against ksh93u+
// 2012-08-01 on 2026-09-11 — #1629.
//
// The chase itself is shared with bash and zsh and the last step is not: a
// name that is *not set* is a parameter reference here, refused with the
// sentence `set -u` writes and fatal with nounset off. The axis comment in
// interp said this shell did not chase at all, which is what made the preset
// look wrong; the preset was right and the comment was describing this.
func TestTheEndOfTheChase(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			name: "a value that names a set variable is followed",
			src:  `y=5; x=y; echo $((x+1))`, want: "6\n",
		},
		{
			name: "and followed as far as the values lead",
			src:  `z=7; y=z; x=y; echo $((x+1))`, want: "8\n",
		},
		{
			name: "a value that names an unset one is refused, and the script stops",
			src:  `x=abc; echo $((x+1)); echo after`,
			want: arithLoc + "abc: parameter not set\n", status: 1,
		},
		{
			name: "a name written in the expression is still a zero",
			src:  `echo $((nosuch+1))`, want: "1\n",
		},
		{
			name: "and so is a value that names a variable holding nothing",
			src:  `y=; x=y; echo $((x+1))`, want: "1\n",
		},
		{
			// The discriminator: with nothing to look up there is no
			// parameter to be unset, and this shell says so differently.
			name: "a value that is no name at all is a syntax error instead",
			src:  `x=1abc; echo $((x+1)); echo after`,
			want: arithLoc + "1abc: arithmetic syntax error\n", status: 1,
		},
		{
			name: "and so is a value with a space in it",
			src:  `x="1 2"; echo $((x+1)); echo after`,
			want: arithLoc + "1 2: arithmetic syntax error\n", status: 1,
		},
		{
			// A refusal `||` cannot catch, which is what makes it the
			// nounset shape rather than a failed command.
			name: "the refusal is not a status a script can branch on",
			src:  `x=abc; echo $((x+1)) || echo caught`,
			want: arithLoc + "abc: parameter not set\n", status: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s\n= %q status %d, want %q status %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}
