// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestASubstitutionsReplacementThatOpensTheResultIsExpanded pins that a `~`
// or `=` a `:s` replacement puts at the head of a field is expanded like one
// written there (#5641), and that one anywhere else, or in a field something
// stands in front of, stays text. Measured 2026-10-03 on zsh 5.9.2 under -f.
func TestASubstitutionsReplacementThatOpensTheResultIsExpanded(t *testing.T) {
	const setup = "HOME=/h; PATH=/bin\n"
	for _, tc := range []struct{ src, want string }{
		{`s=A; print ${s:s/A/~/} ${s:gs/A/~/} ${(U)s:s/A/~/}`, "/h /h /h\n"},
		{`s=A; print ${s:s/A/=ls/}`, "/bin/ls\n"},
		{`a=(A A); print ${a[@]:s/A/~/}; set -- A; print ${@:s/A/~/}`, "/h /h\n/h\n"},
		// Each element after the first opens a field of its own.
		{`a=(A A); print x${a:s/A/~/}; a=(A xA); print ${a:s/A/~/}`, "x~ /h\n/h x~\n"},
		{`a=(A A); print x${a:s/A/=ls/}`, "x=ls /bin/ls\n"},
		// Not at the head of the value, or quoted, it is text.
		{`s=bA; print ${s:s/A/~/}; s=A; print "${s:s/A/~/}" x${s:s/A/~/}`, "b~\n~ x~\n"},
		{`a=(A A); print "x${a[@]:s/A/~/}"`, "x~ ~\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
