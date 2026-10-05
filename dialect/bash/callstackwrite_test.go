// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// An assignment to one of the call-stack arrays, or to PIPESTATUS, is taken
// at status 0 and discarded: the producer goes on answering (#6053).
//
// Measured 2026-10-05 on bash 5.3.20, `env -i PATH=/usr/bin:/bin`, `-c`;
// every expected line is that shell's. This shell stored each write and
// listed it back.
func TestACallStackArrayDiscardsAnAssignment(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`BASH_LINENO=(z); declare -p BASH_LINENO`, "declare -a BASH_LINENO=()\n"},
		{`BASH_SOURCE=(z); echo "${#BASH_SOURCE[@]}"`, "0\n"},
		{`BASH_ARGV=(z); declare -p BASH_ARGV`, "declare -a BASH_ARGV=()\n"},
		{`BASH_ARGC=(z); declare -p BASH_ARGC`, "declare -a BASH_ARGC=([0]=\"0\")\n"},
		{`BASH_ARGV[3]=x; echo $?; declare -p BASH_ARGV`, "0\ndeclare -a BASH_ARGV=()\n"},
		{`BASH_LINENO+=(z); declare -p BASH_LINENO`, "declare -a BASH_LINENO=()\n"},
		{`read -a BASH_ARGV <<< "a b"; declare -p BASH_ARGV`, "declare -a BASH_ARGV=()\n"},
		{`f(){ BASH_LINENO[0]=9; declare -p BASH_LINENO; }; f`, "declare -a BASH_LINENO=([0]=\"1\")\n"},
		{`unset PIPESTATUS; PIPESTATUS=(z); declare -p PIPESTATUS`, "declare -a PIPESTATUS=([0]=\"0\")\n"},
		{`true | false; read -a PIPESTATUS <<< "4 5"; declare -p PIPESTATUS`, "declare -a PIPESTATUS=([0]=\"0\")\n"},
		{`declare -a PIPESTATUS=(z); declare -p PIPESTATUS`, "declare -a PIPESTATUS=([0]=\"0\")\n"},
		// The control: an ordinary array keeps what it was given.
		{`a=(z); declare -p a`, "declare -a a=([0]=\"z\")\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, errs, st := runEmptyArray(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.src, out, errs, st, tc.want)
			}
		})
	}
}
